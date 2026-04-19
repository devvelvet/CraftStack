package process

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// State represents the lifecycle state of a Java process.
type State string

const (
	StateStopped  State = "stopped"
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateStopping State = "stopping"
	StateCrashed  State = "crashed"
)

// InstanceConfig holds the configuration needed to start a Minecraft server.
type InstanceConfig struct {
	ID           string
	Name         string
	JavaPath     string
	MemoryMin    string
	MemoryMax    string
	ServerJar    string
	WorkDir      string
	ExtraJVMArgs []string
}

// LogLine represents a single line of output from the server.
type LogLine struct {
	Timestamp  time.Time
	InstanceID string
	Line       string
}

// JavaProcess manages a single Minecraft server Java process.
type JavaProcess struct {
	config InstanceConfig
	log    *slog.Logger

	mu      sync.RWMutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	state   State
	pid     int
	startAt time.Time

	// Channels
	logCh  chan LogLine
	stopCh chan struct{}

	// streamOutput goroutine complete wait
	outputWg sync.WaitGroup
}

// New creates a new JavaProcess manager.
func New(cfg InstanceConfig, log *slog.Logger) *JavaProcess {
	return &JavaProcess{
		config: cfg,
		log:    log.With("instance", cfg.Name, "id", cfg.ID),
		state:  StateStopped,
		logCh:  make(chan LogLine, 1000),
	}
}

// Name returns the instance name.
func (j *JavaProcess) Name() string {
	return j.config.Name
}

// ID returns the instance ID.
func (j *JavaProcess) ID() string {
	return j.config.ID
}

// InstanceType returns "minecraft" (JavaProcess is used for Minecraft servers).
func (j *JavaProcess) InstanceType() string {
	return "minecraft"
}

// State returns the current process state.
func (j *JavaProcess) State() State {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.state
}

// PID returns the process ID (0 if not running).
func (j *JavaProcess) PID() int {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.pid
}

// Uptime returns how long the process has been running.
func (j *JavaProcess) Uptime() time.Duration {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if j.state != StateRunning {
		return 0
	}
	return time.Since(j.startAt)
}

// LogChannel returns the channel for receiving log lines.
func (j *JavaProcess) LogChannel() <-chan LogLine {
	return j.logCh
}

// Start launches the Java process.
func (j *JavaProcess) Start() error {
	j.mu.Lock()
	if j.state == StateRunning || j.state == StateStarting {
		j.mu.Unlock()
		return fmt.Errorf("instance %s is already %s", j.config.Name, j.state)
	}
	j.state = StateStarting
	j.mu.Unlock()

	// JAR file existing check
	jarPath := j.config.ServerJar
	if j.config.WorkDir != "" {
		jarPath = filepath.Join(j.config.WorkDir, j.config.ServerJar)
	}
	if _, err := os.Stat(jarPath); os.IsNotExist(err) {
		j.setState(StateStopped)
		return fmt.Errorf("server JAR file is missing: %s (please place directly)", jarPath)
	}

	j.log.Info("starting instance")

	// Build JVM arguments
	args := []string{
		fmt.Sprintf("-Xms%s", j.config.MemoryMin),
		fmt.Sprintf("-Xmx%s", j.config.MemoryMax),
	}
	args = append(args, j.config.ExtraJVMArgs...)
	args = append(args, "-jar", j.config.ServerJar, "nogui")

	cmd := exec.Command(j.config.JavaPath, args...)
	cmd.Dir = j.config.WorkDir

	// Inherit some env vars
	cmd.Env = append(os.Environ(), "TERM=xterm")

	// Setup pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		j.setState(StateStopped)
		return fmt.Errorf("create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		j.setState(StateStopped)
		return fmt.Errorf("create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		j.setState(StateStopped)
		return fmt.Errorf("create stderr pipe: %w", err)
	}

	// Start process
	if err := cmd.Start(); err != nil {
		j.setState(StateStopped)
		return fmt.Errorf("start java process: %w", err)
	}

	j.mu.Lock()
	j.cmd = cmd
	j.stdin = stdin
	j.pid = cmd.Process.Pid
	j.state = StateRunning
	j.startAt = time.Now()
	j.stopCh = make(chan struct{})
	j.logCh = make(chan LogLine, 1000) // restart when new channel create
	j.mu.Unlock()

	j.log.Info("instance started", "pid", cmd.Process.Pid)

	// Stream stdout and stderr in goroutines
	j.outputWg.Add(2)
	go j.streamOutput(stdout)
	go j.streamOutput(stderr)

	// Wait for process to exit in background
	go j.waitForExit()

	return nil
}

// Stop gracefully stops the Java process by sending "stop" command.
func (j *JavaProcess) Stop() error {
	j.mu.RLock()
	if j.state != StateRunning {
		j.mu.RUnlock()
		return fmt.Errorf("instance %s is not running (state: %s)", j.config.Name, j.state)
	}
	j.mu.RUnlock()

	j.log.Info("stopping instance")
	j.setState(StateStopping)

	// Send stop command to Minecraft server
	if _, err := j.SendCommand("stop"); err != nil {
		j.log.Warn("failed to send stop command, killing process", "error", err)
		return j.Kill()
	}

	// Wait for graceful shutdown (up to 30 seconds)
	select {
	case <-j.stopCh:
		j.log.Info("instance stopped gracefully")
		return nil
	case <-time.After(30 * time.Second):
		j.log.Warn("graceful shutdown timed out, killing process")
		return j.Kill()
	}
}

// Kill forcefully terminates the process and all its children.
// On Windows, uses taskkill /F /T to kill the entire process tree.
func (j *JavaProcess) Kill() error {
	j.mu.RLock()
	cmd := j.cmd
	pid := j.pid
	j.mu.RUnlock()

	if cmd == nil || cmd.Process == nil {
		return nil
	}

	j.log.Warn("killing process tree", "pid", pid)

	if runtime.GOOS == "windows" {
		// Windows: taskkill /F /T /PID <pid> — process tree all force shutdown
		kill := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid))
		if out, err := kill.CombinedOutput(); err != nil {
			j.log.Warn("taskkill failed, fallback to Process.Kill", "error", err, "output", string(out))
			return cmd.Process.Kill()
		}
		return nil
	}

	// Unix: kill process group (negative PID)
	if err := cmd.Process.Kill(); err != nil {
		return fmt.Errorf("kill process: %w", err)
	}
	return nil
}

// Restart stops and starts the instance.
func (j *JavaProcess) Restart() error {
	if j.State() == StateRunning {
		if err := j.Stop(); err != nil {
			j.log.Warn("error during stop in restart, attempting kill", "error", err)
			j.Kill()
		}
		// Small delay to ensure port is released
		time.Sleep(2 * time.Second)
	}
	return j.Start()
}

// SendCommand writes a command to the Java process stdin.
// For Java processes (Minecraft), the output is not directly capturable from stdin,
// so we return an empty string. The output appears in the log stream instead.
func (j *JavaProcess) SendCommand(command string) (string, error) {
	j.mu.RLock()
	stdin := j.stdin
	state := j.state
	j.mu.RUnlock()

	if state != StateRunning && state != StateStopping {
		return "", fmt.Errorf("cannot send command: instance is %s", state)
	}

	if stdin == nil {
		return "", fmt.Errorf("stdin is not available")
	}

	if !strings.HasSuffix(command, "\n") {
		command += "\n"
	}

	_, err := io.WriteString(stdin, command)
	if err != nil {
		return "", fmt.Errorf("write to stdin: %w", err)
	}

	j.log.Debug("command sent", "command", strings.TrimSpace(command))
	return "", nil
}

// sanitizeUTF8 ensures a string contains only valid UTF-8 characters.
// Minecraft servers may output non-UTF-8 bytes (e.g. legacy encoding, color codes)
// which cause gRPC protobuf string marshaling to fail.
func sanitizeUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	// Replace invalid bytes with U+FFFD (replacement character)
	return strings.ToValidUTF8(s, "\uFFFD")
}

// streamOutput reads lines from the reader and sends them to the log channel.
func (j *JavaProcess) streamOutput(r io.Reader) {
	defer j.outputWg.Done()
	scanner := bufio.NewScanner(r)
	// Increase buffer size for long log lines
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := sanitizeUTF8(scanner.Text())
		select {
		case j.logCh <- LogLine{
			Timestamp:  time.Now(),
			InstanceID: j.config.ID,
			Line:       line,
		}:
		default:
			// Channel full, drop oldest
			select {
			case <-j.logCh:
			default:
			}
			j.logCh <- LogLine{
				Timestamp:  time.Now(),
				InstanceID: j.config.ID,
				Line:       line,
			}
		}
	}
}

// waitForExit waits for the process to exit and updates state accordingly.
func (j *JavaProcess) waitForExit() {
	err := j.cmd.Wait()

	// streamOutput goroutine wait until all have ended (logCh no more writes guarantee)
	j.outputWg.Wait()

	j.mu.Lock()
	prevState := j.state
	if err != nil && prevState != StateStopping {
		j.state = StateCrashed
		j.log.Error("instance crashed", "error", err)
	} else {
		j.state = StateStopped
		j.log.Info("instance exited")
	}
	j.pid = 0
	j.cmd = nil
	j.stdin = nil
	stopCh := j.stopCh
	logCh := j.logCh
	j.mu.Unlock()

	// logCh close forwardLogs process shutdown detectcan 
	// outputWg.Wait() afterwards, so send on closed channel panic none
	if logCh != nil {
		close(logCh)
	}

	// Signal that process has stopped
	if stopCh != nil {
		close(stopCh)
	}
}

// setState safely updates the state.
func (j *JavaProcess) setState(s State) {
	j.mu.Lock()
	j.state = s
	j.mu.Unlock()
}

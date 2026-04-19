package process

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// DockerConfig holds the configuration for a Docker-managed process.
type DockerConfig struct {
	ID            string // instance ID
	Name          string // instance name
	Type          string // instance type (minecraft, mysql, postgresql, mongodb, redis, kafka)
	ContainerName string // Docker container name (e.g.: craftstack-survival)
	DockerPath    string // docker binary path
}

// DockerProcess manages a Docker container as a Process.
// Implements the Process interface.
type DockerProcess struct {
	config DockerConfig
	log    *slog.Logger

	mu        sync.RWMutex
	state     State
	pid       int // container PID (host basis)
	startAt   time.Time
	logCh     chan LogLine
	stopCh    chan struct{}
	logCancel context.CancelFunc
	outputWg  sync.WaitGroup
}

// NewDocker creates a new DockerProcess manager.
func NewDocker(cfg DockerConfig, log *slog.Logger) *DockerProcess {
	if cfg.DockerPath == "" {
		cfg.DockerPath = "docker"
	}

	return &DockerProcess{
		config: cfg,
		log:    log.With("instance", cfg.Name, "id", cfg.ID, "type", cfg.Type, "container", cfg.ContainerName),
		state:  StateStopped,
		logCh:  make(chan LogLine, 1000),
	}
}

// ID returns the instance ID.
func (d *DockerProcess) ID() string { return d.config.ID }

// Name returns the instance name.
func (d *DockerProcess) Name() string { return d.config.Name }

// InstanceType returns the service type.
func (d *DockerProcess) InstanceType() string { return d.config.Type }

// State returns the current process state.
func (d *DockerProcess) State() State {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.state
}

// PID returns the container's main process PID on the host (0 if not running).
func (d *DockerProcess) PID() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.pid
}

// Uptime returns how long the container has been running.
func (d *DockerProcess) Uptime() time.Duration {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.state != StateRunning {
		return 0
	}
	return time.Since(d.startAt)
}

// LogChannel returns the channel for receiving log lines.
func (d *DockerProcess) LogChannel() <-chan LogLine {
	return d.logCh
}

// Start starts the Docker container.
func (d *DockerProcess) Start() error {
	d.mu.Lock()
	if d.state == StateRunning || d.state == StateStarting {
		d.mu.Unlock()
		return fmt.Errorf("instance %s is already %s", d.config.Name, d.state)
	}
	d.state = StateStarting
	d.mu.Unlock()

	d.log.Info("Docker start container", "container", d.config.ContainerName)

	// docker start
	cmd := exec.Command(d.config.DockerPath, "start", d.config.ContainerName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		d.setState(StateStopped)
		return fmt.Errorf("docker start failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	// PID query
	pid := d.fetchPID()

	d.mu.Lock()
	d.state = StateRunning
	d.pid = pid
	d.startAt = time.Now()
	d.stopCh = make(chan struct{})
	d.logCh = make(chan LogLine, 1000)
	d.mu.Unlock()

	d.log.Info("Docker start container", "pid", pid)

	// log streaming start
	d.startLogStreaming()

	// container shutdown watch
	go d.waitForExit()

	return nil
}

// Stop gracefully stops the Docker container.
func (d *DockerProcess) Stop() error {
	d.mu.RLock()
	if d.state != StateRunning {
		d.mu.RUnlock()
		return fmt.Errorf("instance %s is not running (state: %s)", d.config.Name, d.state)
	}
	d.mu.RUnlock()

	d.log.Info("Docker stop container")
	d.setState(StateStopping)

	cmd := exec.Command(d.config.DockerPath, "stop", "-t", "30", d.config.ContainerName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		d.log.Warn("docker stop failed, kill attempt", "error", err, "output", string(out))
		return d.Kill()
	}

	// waitForExit from state update
	select {
	case <-d.stopCh:
		d.log.Info("Docker container normal stopped")
		return nil
	case <-time.After(35 * time.Second):
		d.log.Warn("stop container timeout, kill attempt")
		return d.Kill()
	}
}

// Kill forcefully terminates the Docker container.
func (d *DockerProcess) Kill() error {
	d.log.Warn("Docker container force shutdown", "container", d.config.ContainerName)

	cmd := exec.Command(d.config.DockerPath, "kill", d.config.ContainerName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		d.log.Warn("docker kill failed", "error", err, "output", string(out))
		// already shutdown caseday may
	}

	d.mu.Lock()
	d.state = StateStopped
	d.pid = 0
	d.mu.Unlock()

	return nil
}

// Restart stops and starts the container.
func (d *DockerProcess) Restart() error {
	if d.State() == StateRunning {
		if err := d.Stop(); err != nil {
			d.log.Warn("restart during stop failed, kill attempt", "error", err)
			d.Kill()
		}
		time.Sleep(2 * time.Second)
	}
	return d.Start()
}

// SendCommand sends a command to the container's stdin.
// For Minecraft: docker exec -i container rcon-cli or stdin
// For Redis: docker exec container redis-cli COMMAND
// For others: docker exec
func (d *DockerProcess) SendCommand(command string) (string, error) {
	d.mu.RLock()
	state := d.state
	d.mu.RUnlock()

	if state != StateRunning && state != StateStopping {
		return "", fmt.Errorf("cannot send command: instance is %s", state)
	}

	// command sending format per service type
	switch d.config.Type {
	case "minecraft":
		// Minecraft: docker exec as rcon-cli use or attach stdin
		return d.sendMinecraftCommand(command)
	case "redis":
		// Redis: docker exec redis-cli use
		return d.sendRedisCommand(command)
	case "mysql":
		// MySQL: docker exec mysql use
		return d.sendMySQLCommand(command)
	case "postgresql":
		// PostgreSQL: docker exec psql use
		return d.sendPostgreSQLCommand(command)
	case "mongodb":
		// MongoDB: docker exec mongosh use
		return d.sendMongoCommand(command)
	default:
		// general  docker exec sh -c "command"
		cmd := exec.Command(d.config.DockerPath, "exec", d.config.ContainerName, "sh", "-c", command)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("command execution failed: %s: %w", strings.TrimSpace(string(out)), err)
		}
		return strings.TrimSpace(string(out)), nil
	}
}

func (d *DockerProcess) sendMinecraftCommand(command string) (string, error) {
	// Minecraft server stdin as send command
	// docker exec -i container sh -c "echo 'command' > /proc/1/fd/0" format is not used
	// new docker attach stdin as forward
	cmd := exec.Command(d.config.DockerPath, "exec", "-i", d.config.ContainerName,
		"sh", "-c", fmt.Sprintf("echo '%s' > /proc/1/fd/0", command))
	out, err := cmd.CombinedOutput()
	if err != nil {
		// fallback: rcon-cli if use
		rconCmd := exec.Command(d.config.DockerPath, "exec", d.config.ContainerName,
			"rcon-cli", command)
		if rconOut, rconErr := rconCmd.CombinedOutput(); rconErr == nil {
			return strings.TrimSpace(string(rconOut)), nil
		}
		return "", fmt.Errorf("Minecraft send command failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (d *DockerProcess) sendRedisCommand(command string) (string, error) {
	parts := strings.Fields(command)
	// first password without attempt
	args := []string{"exec", d.config.ContainerName, "redis-cli"}
	args = append(args, parts...)
	cmd := exec.Command(d.config.DockerPath, args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return strings.TrimSpace(string(out)), nil
	}

	// NOAUTH or auth error → REDIS_PASSWORD environment variable use
	outStr := string(out)
	if strings.Contains(outStr, "NOAUTH") || strings.Contains(outStr, "AUTH") {
		shellCmd := fmt.Sprintf(`redis-cli -a "$REDIS_PASSWORD" --no-auth-warning %s`, command)
		cmd2 := exec.Command(d.config.DockerPath, "exec", d.config.ContainerName, "sh", "-c", shellCmd)
		out2, err2 := cmd2.CombinedOutput()
		if err2 != nil {
			return "", fmt.Errorf("Redis command failed: %s: %w", strings.TrimSpace(string(out2)), err2)
		}
		return strings.TrimSpace(string(out2)), nil
	}

	return "", fmt.Errorf("Redis command failed: %s: %w", strings.TrimSpace(outStr), err)
}

func (d *DockerProcess) sendMySQLCommand(command string) (string, error) {
	// first password without attempt
	cmd := exec.Command(d.config.DockerPath, "exec", d.config.ContainerName,
		"mysql", "-u", "root", "-e", command)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return strings.TrimSpace(string(out)), nil
	}

	// Access denied → container MYSQL_ROOT_PASSWORD environment variable use
	if strings.Contains(string(out), "Access denied") {
		shellCmd := fmt.Sprintf(`mysql -u root -p"$MYSQL_ROOT_PASSWORD" -e %s`, shellescape(command))
		cmd2 := exec.Command(d.config.DockerPath, "exec", d.config.ContainerName, "sh", "-c", shellCmd)
		out2, err2 := cmd2.CombinedOutput()
		if err2 != nil {
			return "", fmt.Errorf("MySQL command failed: %s: %w", strings.TrimSpace(string(out2)), err2)
		}
		return strings.TrimSpace(string(out2)), nil
	}

	return "", fmt.Errorf("MySQL command failed: %s: %w", strings.TrimSpace(string(out)), err)
}

func (d *DockerProcess) sendPostgreSQLCommand(command string) (string, error) {
	// PGPASSWORD environment variable use password auth handle
	shellCmd := fmt.Sprintf(`PGPASSWORD="$POSTGRES_PASSWORD" psql -U postgres -c %s`, shellescape(command))
	cmd := exec.Command(d.config.DockerPath, "exec", d.config.ContainerName, "sh", "-c", shellCmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// fallback: password without attempt (trust auth)
		cmd2 := exec.Command(d.config.DockerPath, "exec", d.config.ContainerName,
			"psql", "-U", "postgres", "-c", command)
		out2, err2 := cmd2.CombinedOutput()
		if err2 != nil {
			return "", fmt.Errorf("PostgreSQL command failed: %s: %w", strings.TrimSpace(string(out)), err)
		}
		return strings.TrimSpace(string(out2)), nil
	}
	return strings.TrimSpace(string(out)), nil
}

func (d *DockerProcess) sendMongoCommand(command string) (string, error) {
	// first auth without attempt
	cmd := exec.Command(d.config.DockerPath, "exec", d.config.ContainerName,
		"mongosh", "--eval", command)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return strings.TrimSpace(string(out)), nil
	}

	// authentication failed → environment variable as auth attempt
	if strings.Contains(string(out), "auth") || strings.Contains(string(out), "Unauthorized") {
		shellCmd := fmt.Sprintf(`mongosh -u "$MONGO_INITDB_ROOT_USERNAME" -p "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin --eval %s`, shellescape(command))
		cmd2 := exec.Command(d.config.DockerPath, "exec", d.config.ContainerName, "sh", "-c", shellCmd)
		out2, err2 := cmd2.CombinedOutput()
		if err2 != nil {
			return "", fmt.Errorf("MongoDB command failed: %s: %w", strings.TrimSpace(string(out2)), err2)
		}
		return strings.TrimSpace(string(out2)), nil
	}

	return "", fmt.Errorf("MongoDB command failed: %s: %w", strings.TrimSpace(string(out)), err)
}

// fetchPID gets the container's main process PID on the host.
func (d *DockerProcess) fetchPID() int {
	cmd := exec.Command(d.config.DockerPath, "inspect", "--format", "{{.State.Pid}}", d.config.ContainerName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0
	}
	pid := 0
	fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &pid)
	return pid
}

// startLogStreaming starts following container logs in a goroutine.
func (d *DockerProcess) startLogStreaming() {
	ctx, cancel := context.WithCancel(context.Background())

	d.mu.Lock()
	d.logCancel = cancel
	d.mu.Unlock()

	d.outputWg.Add(1)
	go func() {
		defer d.outputWg.Done()
		d.streamLogs(ctx)
	}()
}

// streamLogs follows docker logs and sends lines to logCh.
func (d *DockerProcess) streamLogs(ctx context.Context) {
	cmd := exec.CommandContext(ctx, d.config.DockerPath, "logs", "-f", "--tail", "100", d.config.ContainerName)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		d.log.Warn("docker logs stdout pipe failed", "error", err)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		d.log.Warn("docker logs stderr pipe failed", "error", err)
		return
	}

	if err := cmd.Start(); err != nil {
		d.log.Warn("docker logs start failed", "error", err)
		return
	}

	// merge stdout/stderr and read
	var wg sync.WaitGroup
	readPipe := func(r io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := sanitizeUTF8(scanner.Text())
			select {
			case d.logCh <- LogLine{
				Timestamp:  time.Now(),
				InstanceID: d.config.ID,
				Line:       line,
			}:
			default:
				// if the channel fills, drop the oldest
				select {
				case <-d.logCh:
				default:
				}
				d.logCh <- LogLine{
					Timestamp:  time.Now(),
					InstanceID: d.config.ID,
					Line:       line,
				}
			}
		}
	}

	wg.Add(2)
	go readPipe(stdout)
	go readPipe(stderr)
	wg.Wait()

	cmd.Wait()
}

// waitForExit monitors the container and updates state when it exits.
func (d *DockerProcess) waitForExit() {
	// docker wait: block until container shuts down
	cmd := exec.Command(d.config.DockerPath, "wait", d.config.ContainerName)
	out, err := cmd.CombinedOutput()

	// log streaming cancel
	d.mu.RLock()
	if d.logCancel != nil {
		d.logCancel()
	}
	d.mu.RUnlock()

	// output wait
	d.outputWg.Wait()

	d.mu.Lock()
	prevState := d.state

	exitCode := 0
	if err == nil {
		fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &exitCode)
	}

	if exitCode != 0 && prevState != StateStopping {
		d.state = StateCrashed
		d.log.Error("Docker container abnormal shutdown", "exit_code", exitCode)
	} else {
		d.state = StateStopped
		d.log.Info("Docker container shutdown", "exit_code", exitCode)
	}

	d.pid = 0
	stopCh := d.stopCh
	logCh := d.logCh
	d.mu.Unlock()

	if logCh != nil {
		close(logCh)
	}
	if stopCh != nil {
		close(stopCh)
	}
}

// setState safely updates the state.
func (d *DockerProcess) setState(s State) {
	d.mu.Lock()
	d.state = s
	d.mu.Unlock()
}

// shellescape wraps a string in single quotes for safe use in sh -c commands.
// Internal single quotes are escaped via the standard '\” pattern.
func shellescape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// RefreshState checks the actual container state from Docker and updates accordingly.
// Called during initial sync to detect already-running containers.
func (d *DockerProcess) RefreshState() {
	cmd := exec.Command(d.config.DockerPath, "inspect", "--format", "{{.State.Status}}", d.config.ContainerName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		d.setState(StateStopped)
		return
	}

	status := strings.TrimSpace(string(out))
	switch status {
	case "running":
		pid := d.fetchPID()
		d.mu.Lock()
		d.state = StateRunning
		d.pid = pid
		d.startAt = time.Now() //  use approximate start time from inspect
		d.stopCh = make(chan struct{})
		d.logCh = make(chan LogLine, 1000)
		d.mu.Unlock()
		d.startLogStreaming()
		go d.waitForExit()
	case "exited", "dead":
		d.setState(StateStopped)
	case "created":
		d.setState(StateStopped)
	default:
		d.setState(StateStopped)
	}
}

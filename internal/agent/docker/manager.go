// Package docker provides Docker container management for CraftStack agent.
// Uses the Docker CLI directly (no SDK) to maintain CGO_ENABLED=0 compatibility.
package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// ContainerInfo holds information about a running Docker container.
type ContainerInfo struct {
	ID      string `json:"Id"`
	Name    string `json:"Name"`
	State   string `json:"State"`
	Status  string `json:"Status"`
	Image   string `json:"Image"`
	Created string `json:"Created"`
}

// Manager handles Docker operations via CLI.
type Manager struct {
	log        *slog.Logger
	dockerPath string // docker binary path
}

// NewManager creates a new Docker manager.
func NewManager(log *slog.Logger) *Manager {
	dockerPath := "docker"
	if p, err := exec.LookPath("docker"); err == nil {
		dockerPath = p
	}

	return &Manager{
		log:        log,
		dockerPath: dockerPath,
	}
}

// DockerPath returns the path to the docker binary.
func (m *Manager) DockerPath() string {
	return m.dockerPath
}

// IsInstalled checks if docker CLI binary exists on the system.
// This does NOT check if the daemon is running.
func (m *Manager) IsInstalled() bool {
	// docker --version server connection unnecessary — CLIonly if present success
	cmd := exec.Command(m.dockerPath, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		m.log.Debug("Docker CLI check failed", "error", err, "output", string(out))
		return false
	}
	return strings.Contains(string(out), "Docker")
}

// IsRunning checks if Docker daemon is running and responsive.
func (m *Manager) IsRunning() bool {
	cmd := exec.Command(m.dockerPath, "info", "--format", "{{.ServerVersion}}")
	cmd.Stdin = nil
	// short timeout for fast failure
	out, err := cmd.CombinedOutput()
	if err != nil {
		m.log.Debug("Docker daemon check failed", "error", err, "output", string(out))
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

// PullImage pulls a Docker image.
func (m *Manager) PullImage(ctx context.Context, image string) error {
	m.log.Info("Docker image pool", "image", image)
	cmd := exec.CommandContext(ctx, m.dockerPath, "pull", image)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker pull failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	m.log.Info("Docker image pool complete", "image", image)
	return nil
}

// ContainerConfig holds the configuration for creating a Docker container.
type ContainerConfig struct {
	Name        string            // container name
	Image       string            // Docker image
	Ports       map[int]int       // host:container port mapping
	Volumes     map[string]string // host:container volume mapping
	Env         map[string]string // environment variable
	Cmd         []string          // container command override
	RestartMode string            // restart policy: "no", "always", "unless-stopped"
	Memory      string            // memory limit (e.g.: "1g", "512m")
	CPUs        string            // CPU limit (e.g.: "2.0", "0.5")
	WorkDir     string            // container my work directory
	Network     string            // Docker network name (if empty default bridge)
	DNS         []string          // --dns flag (embedded DNS server address)
}

// CreateContainer creates a Docker container with the given config.
// Returns the container ID.
func (m *Manager) CreateContainer(ctx context.Context, cfg ContainerConfig) (string, error) {
	args := []string{"create", "--name", cfg.Name}

	// port mapping
	for host, container := range cfg.Ports {
		args = append(args, "-p", fmt.Sprintf("%d:%d", host, container))
	}

	// volume mapping
	for host, container := range cfg.Volumes {
		args = append(args, "-v", fmt.Sprintf("%s:%s", host, container))
	}

	// environment variable
	for k, v := range cfg.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	// restart policy
	if cfg.RestartMode != "" {
		args = append(args, "--restart", cfg.RestartMode)
	}

	// memory limit
	if cfg.Memory != "" {
		args = append(args, "--memory", cfg.Memory)
	}

	// CPU limit
	if cfg.CPUs != "" {
		args = append(args, "--cpus", cfg.CPUs)
	}

	// work directory
	if cfg.WorkDir != "" {
		args = append(args, "-w", cfg.WorkDir)
	}

	// Docker network
	if cfg.Network != "" {
		args = append(args, "--network", cfg.Network)
	}

	// DNS server (cross-node service discovery)
	for _, dns := range cfg.DNS {
		args = append(args, "--dns", dns)
	}

	// stdin connect (needed for Redis etc. stdin commands)
	args = append(args, "-i")

	// image
	args = append(args, cfg.Image)

	// command override
	if len(cfg.Cmd) > 0 {
		args = append(args, cfg.Cmd...)
	}

	m.log.Info("Docker create container", "name", cfg.Name, "image", cfg.Image)
	cmd := exec.CommandContext(ctx, m.dockerPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker create failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	containerID := strings.TrimSpace(string(out))
	m.log.Info("Docker create container complete", "name", cfg.Name, "id", containerID[:12])
	return containerID, nil
}

// StartContainer starts a Docker container.
func (m *Manager) StartContainer(ctx context.Context, nameOrID string) error {
	cmd := exec.CommandContext(ctx, m.dockerPath, "start", nameOrID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker start failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// StopContainer stops a Docker container with a timeout.
func (m *Manager) StopContainer(ctx context.Context, nameOrID string, timeout int) error {
	args := []string{"stop"}
	if timeout > 0 {
		args = append(args, "-t", fmt.Sprintf("%d", timeout))
	}
	args = append(args, nameOrID)

	cmd := exec.CommandContext(ctx, m.dockerPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker stop failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// KillContainer forcefully kills a Docker container.
func (m *Manager) KillContainer(ctx context.Context, nameOrID string) error {
	cmd := exec.CommandContext(ctx, m.dockerPath, "kill", nameOrID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker kill failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// RemoveContainer removes a Docker container.
func (m *Manager) RemoveContainer(ctx context.Context, nameOrID string, force bool) error {
	args := []string{"rm"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, nameOrID)

	cmd := exec.CommandContext(ctx, m.dockerPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker rm failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// ExecCommand executes a command inside a running container.
func (m *Manager) ExecCommand(ctx context.Context, nameOrID string, command []string) (string, error) {
	args := []string{"exec", nameOrID}
	args = append(args, command...)

	cmd := exec.CommandContext(ctx, m.dockerPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker exec failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return string(out), nil
}

// InspectContainer returns detailed information about a container.
func (m *Manager) InspectContainer(ctx context.Context, nameOrID string) (*InspectResult, error) {
	cmd := exec.CommandContext(ctx, m.dockerPath, "inspect", nameOrID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker inspect failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	var results []InspectResult
	if err := json.Unmarshal(out, &results); err != nil {
		return nil, fmt.Errorf("docker inspect parse failed: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("container not found: %s", nameOrID)
	}
	return &results[0], nil
}

// InspectResult is a subset of docker inspect output.
type InspectResult struct {
	ID    string `json:"Id"`
	State struct {
		Status     string `json:"Status"` // created, running, paused, restarting, removing, exited, dead
		Running    bool   `json:"Running"`
		Pid        int    `json:"Pid"`
		ExitCode   int    `json:"ExitCode"`
		StartedAt  string `json:"StartedAt"`
		FinishedAt string `json:"FinishedAt"`
	} `json:"State"`
	Config struct {
		Image string `json:"Image"`
	} `json:"Config"`
}

// ContainerExists checks if a container with the given name or ID exists.
func (m *Manager) ContainerExists(ctx context.Context, nameOrID string) bool {
	tCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(tCtx, m.dockerPath, "inspect", "--type", "container", nameOrID)
	return cmd.Run() == nil
}

// GetContainerPID returns the PID of the container's main process on the host.
func (m *Manager) GetContainerPID(ctx context.Context, nameOrID string) (int, error) {
	info, err := m.InspectContainer(ctx, nameOrID)
	if err != nil {
		return 0, err
	}
	return info.State.Pid, nil
}

// GetContainerStatus returns the container's status string.
func (m *Manager) GetContainerStatus(ctx context.Context, nameOrID string) (string, error) {
	cmd := exec.CommandContext(ctx, m.dockerPath, "inspect", "--format", "{{.State.Status}}", nameOrID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("state query failed: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// LogsFollow starts following container logs and returns stdout.
// Caller must close the returned process when done.
func (m *Manager) LogsFollow(ctx context.Context, nameOrID string) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, m.dockerPath, "logs", "-f", "--tail", "100", nameOrID)
	return cmd, nil
}

// NetworkCreate creates a Docker network with optional driver, subnet, and gateway.
func (m *Manager) NetworkCreate(ctx context.Context, name, driver, subnet, gateway string) (string, error) {
	if driver == "" {
		driver = "bridge"
	}

	args := []string{"network", "create", "--driver", driver}

	if subnet != "" {
		args = append(args, "--subnet", subnet)
	}
	if gateway != "" {
		args = append(args, "--gateway", gateway)
	}

	args = append(args, name)

	cmd := exec.CommandContext(ctx, m.dockerPath, args...)
	out, err := cmd.CombinedOutput()
	outStr := strings.TrimSpace(string(out))
	if err != nil {
		// already existing ignore
		if strings.Contains(outStr, "already exists") {
			return "", nil
		}
		return "", fmt.Errorf("docker network create failed: %s: %w", outStr, err)
	}
	m.log.Info("Docker create network complete", "name", name, "driver", driver)
	return outStr, nil
}

// NetworkRemove removes a Docker network.
func (m *Manager) NetworkRemove(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, m.dockerPath, "network", "rm", name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		outStr := strings.TrimSpace(string(out))
		if strings.Contains(outStr, "not found") {
			return nil
		}
		return fmt.Errorf("docker network rm failed: %s: %w", outStr, err)
	}
	m.log.Info("Docker delete network complete", "name", name)
	return nil
}

// NetworkConnect connects a container to a Docker network.
func (m *Manager) NetworkConnect(ctx context.Context, networkName, containerName, alias, ipAddress string) error {
	args := []string{"network", "connect"}
	if alias != "" {
		args = append(args, "--alias", alias)
	}
	if ipAddress != "" {
		args = append(args, "--ip", ipAddress)
	}
	args = append(args, networkName, containerName)

	cmd := exec.CommandContext(ctx, m.dockerPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		outStr := strings.TrimSpace(string(out))
		// already connect if present ignore
		if strings.Contains(outStr, "already exists") {
			return nil
		}
		return fmt.Errorf("docker network connect failed: %s: %w", outStr, err)
	}
	m.log.Info("Docker connect network", "network", networkName, "container", containerName)
	return nil
}

// NetworkDisconnect disconnects a from containers a Docker network.
func (m *Manager) NetworkDisconnect(ctx context.Context, networkName, containerName string) error {
	cmd := exec.CommandContext(ctx, m.dockerPath, "network", "disconnect", networkName, containerName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		outStr := strings.TrimSpace(string(out))
		if strings.Contains(outStr, "is not connected") {
			return nil
		}
		return fmt.Errorf("docker network disconnect failed: %s: %w", outStr, err)
	}
	m.log.Info("Docker connect network release", "network", networkName, "container", containerName)
	return nil
}

// NetworkInfo holds parsed Docker network inspection data.
type NetworkInfo struct {
	ID     string `json:"Id"`
	Name   string `json:"Name"`
	Driver string `json:"Driver"`
	Scope  string `json:"Scope"`
	IPAM   struct {
		Config []struct {
			Subnet  string `json:"Subnet"`
			Gateway string `json:"Gateway"`
		} `json:"Config"`
	} `json:"IPAM"`
	Containers map[string]struct {
		Name string `json:"Name"`
	} `json:"Containers"`
}

// NetworkList lists all Docker networks (excluding default system networks).
func (m *Manager) NetworkList(ctx context.Context) ([]NetworkInfo, error) {
	cmd := exec.CommandContext(ctx, m.dockerPath, "network", "ls", "--format", "{{.Name}}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker network ls failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name := strings.TrimSpace(line)
		if name != "" {
			names = append(names, name)
		}
	}

	var networks []NetworkInfo
	for _, name := range names {
		info, err := m.NetworkInspect(ctx, name)
		if err != nil {
			m.log.Debug("network inspect failed (ignore)", "name", name, "error", err)
			continue
		}
		networks = append(networks, *info)
	}

	return networks, nil
}

// NetworkInspect returns detailed information about a Docker network.
func (m *Manager) NetworkInspect(ctx context.Context, name string) (*NetworkInfo, error) {
	cmd := exec.CommandContext(ctx, m.dockerPath, "network", "inspect", name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker network inspect failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	var results []NetworkInfo
	if err := json.Unmarshal(out, &results); err != nil {
		return nil, fmt.Errorf("docker network inspect parse failed: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("network not found: %s", name)
	}
	return &results[0], nil
}

// Version returns the Docker version info string.
func (m *Manager) Version(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, m.dockerPath, "version", "--format",
		"Client: {{.Client.Version}}, Server: {{.Server.Version}}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker version failed: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// AttachStdin attaches stdin to a running container for sending commands.
// Returns the exec.Cmd so caller can write to its Stdin pipe.
func (m *Manager) AttachStdin(ctx context.Context, nameOrID string) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, m.dockerPath, "attach",
		"--no-stdin=false", "--sig-proxy=false", nameOrID)
	return cmd, nil
}

// WaitContainer waits for a container to stop.
func (m *Manager) WaitContainer(ctx context.Context, nameOrID string, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(waitCtx, m.dockerPath, "wait", nameOrID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker wait failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// GetDockerBinaryPath returns the path to the docker binary, checking common locations.
func GetDockerBinaryPath() string {
	if p, err := exec.LookPath("docker"); err == nil {
		return p
	}

	// common install path check
	var candidates []string
	if runtime.GOOS == "windows" {
		candidates = []string{
			`C:\Program Files\Docker\Docker\resources\bin\docker.exe`,
			`C:\ProgramData\DockerDesktop\version-bin\docker.exe`,
		}
	} else {
		candidates = []string{
			"/usr/bin/docker",
			"/usr/local/bin/docker",
			"/snap/bin/docker",
		}
	}

	for _, p := range candidates {
		cmd := exec.Command(p, "version")
		if cmd.Run() == nil {
			return p
		}
	}

	return "docker"
}

// BuildImage builds a Docker image from a Dockerfile in the given directory.
func (m *Manager) BuildImage(ctx context.Context, contextDir, tag string) error {
	args := []string{"build", "-t", tag, contextDir}
	m.log.Info("Docker image build", "tag", tag, "context", contextDir)
	cmd := exec.CommandContext(ctx, m.dockerPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker build failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	m.log.Info("Docker image build complete", "tag", tag)
	return nil
}

// ComposeUp runs docker compose up -d in the given directory.
func (m *Manager) ComposeUp(ctx context.Context, workDir string) error {
	// docker compose (v2) or docker-compose (v1) attempt
	args := []string{"compose", "up", "-d"}
	m.log.Info("docker compose up", "workdir", workDir)
	cmd := exec.CommandContext(ctx, m.dockerPath, args...)
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		// v2 failed when docker-compose (v1) attempt
		cmd2 := exec.CommandContext(ctx, "docker-compose", "up", "-d")
		cmd2.Dir = workDir
		out2, err2 := cmd2.CombinedOutput()
		if err2 != nil {
			return fmt.Errorf("docker compose up failed: %s / %s: %w", strings.TrimSpace(string(out)), strings.TrimSpace(string(out2)), err2)
		}
	}
	m.log.Info("docker compose up complete", "workdir", workDir)
	return nil
}

// ComposeDown runs docker compose down in the given directory.
func (m *Manager) ComposeDown(ctx context.Context, workDir string) error {
	args := []string{"compose", "down"}
	cmd := exec.CommandContext(ctx, m.dockerPath, args...)
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		cmd2 := exec.CommandContext(ctx, "docker-compose", "down")
		cmd2.Dir = workDir
		out2, err2 := cmd2.CombinedOutput()
		if err2 != nil {
			return fmt.Errorf("docker compose down failed: %s / %s: %w", strings.TrimSpace(string(out)), strings.TrimSpace(string(out2)), err2)
		}
	}
	return nil
}

// ContainerStats holds runtime resource usage for a Docker container.
type ContainerStats struct {
	CPUPercent      float64
	MemoryUsageMB   int64
	MemoryLimitMB   int64
	NetRxBytes      int64
	NetTxBytes      int64
	BlockReadBytes  int64
	BlockWriteBytes int64
}

// Stats returns resource usage stats for a container using `docker stats --no-stream --format`.
// Returns nil, nil if the container is not running (does not error).
func (m *Manager) Stats(ctx context.Context, containerName string) (*ContainerStats, error) {
	// Quick check: container must be running
	status, err := m.GetContainerStatus(ctx, containerName)
	if err != nil || status != "running" {
		return nil, nil
	}

	cmd := exec.CommandContext(ctx, m.dockerPath, "stats", "--no-stream", "--format",
		"{{.CPUPerc}}|{{.MemUsage}}|{{.NetIO}}|{{.BlockIO}}", containerName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		outStr := strings.TrimSpace(string(out))
		// Container might have stopped between check and stats call
		if strings.Contains(outStr, "No such container") || strings.Contains(outStr, "is not running") {
			return nil, nil
		}
		return nil, fmt.Errorf("docker stats failed: %s: %w", outStr, err)
	}

	line := strings.TrimSpace(string(out))
	if line == "" {
		return nil, nil
	}

	parts := strings.SplitN(line, "|", 4)
	if len(parts) < 4 {
		return nil, fmt.Errorf("docker stats output parse failed: %s", line)
	}

	stats := &ContainerStats{}

	// Parse CPU: "1.23%"
	cpuStr := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(parts[0]), "%"))
	if v, err := parseFloat(cpuStr); err == nil {
		stats.CPUPercent = v
	}

	// Parse MemUsage: "256MiB / 1GiB"
	memParts := strings.SplitN(strings.TrimSpace(parts[1]), "/", 2)
	if len(memParts) == 2 {
		stats.MemoryUsageMB = parseSizeToMB(strings.TrimSpace(memParts[0]))
		stats.MemoryLimitMB = parseSizeToMB(strings.TrimSpace(memParts[1]))
	}

	// Parse NetIO: "1.45kB / 648B"
	netParts := strings.SplitN(strings.TrimSpace(parts[2]), "/", 2)
	if len(netParts) == 2 {
		stats.NetRxBytes = parseSize(strings.TrimSpace(netParts[0]))
		stats.NetTxBytes = parseSize(strings.TrimSpace(netParts[1]))
	}

	// Parse BlockIO: "12.3MB / 0B"
	blockParts := strings.SplitN(strings.TrimSpace(parts[3]), "/", 2)
	if len(blockParts) == 2 {
		stats.BlockReadBytes = parseSize(strings.TrimSpace(blockParts[0]))
		stats.BlockWriteBytes = parseSize(strings.TrimSpace(blockParts[1]))
	}

	return stats, nil
}

// parseFloat parses a float string, ignoring errors gracefully.
func parseFloat(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "--" {
		return 0, fmt.Errorf("empty")
	}
	var v float64
	_, err := fmt.Sscanf(s, "%f", &v)
	return v, err
}

// parseSize parses a Docker size string like "1.45kB", "256MiB", "1GiB", "0B" into bytes.
func parseSize(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "--" {
		return 0
	}

	// Map of unit suffixes to their byte multiplier
	type unitDef struct {
		suffix     string
		multiplier float64
	}
	units := []unitDef{
		{"GiB", 1024 * 1024 * 1024},
		{"MiB", 1024 * 1024},
		{"KiB", 1024},
		{"GB", 1000 * 1000 * 1000},
		{"MB", 1000 * 1000},
		{"KB", 1000},
		{"kB", 1000},
		{"B", 1},
	}

	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			numStr := strings.TrimSpace(strings.TrimSuffix(s, u.suffix))
			if numStr == "" {
				return 0
			}
			var v float64
			if _, err := fmt.Sscanf(numStr, "%f", &v); err == nil {
				return int64(v * u.multiplier)
			}
			return 0
		}
	}

	// Try parsing as plain number (bytes)
	var v float64
	if _, err := fmt.Sscanf(s, "%f", &v); err == nil {
		return int64(v)
	}
	return 0
}

// parseSizeToMB parses a Docker size string and returns megabytes.
func parseSizeToMB(s string) int64 {
	bytes := parseSize(s)
	return bytes / (1024 * 1024)
}

// SendToStdin sends a command to a container's stdin using docker exec.
// This is useful for services like Redis that accept stdin commands.
func (m *Manager) SendToStdin(ctx context.Context, nameOrID string, input string) error {
	cmd := exec.CommandContext(ctx, m.dockerPath, "exec", "-i", nameOrID, "sh", "-c", "cat")
	cmd.Stdin = bytes.NewBufferString(input + "\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("stdin send failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

package docker

import (
	"context"
	"fmt"
	"log/slog"
)

// InstallResult preserved for API compatibility with callers that report status.
type InstallResult struct {
	Installed bool   `json:"installed"`
	Version   string `json:"version"`
	Message   string `json:"message"`
}

// CheckAndInstall verifies Docker is present. CraftStack does not auto-install
// dependencies; operators must install Docker manually.
func CheckAndInstall(ctx context.Context, log *slog.Logger) (*InstallResult, error) {
	mgr := NewManager(log)
	if !mgr.IsInstalled() {
		return &InstallResult{Installed: false, Message: "Docker not installed"},
			fmt.Errorf("docker is not installed on this host; install Docker manually before running the agent")
	}
	if !mgr.IsRunning() {
		return &InstallResult{Installed: true, Message: "Docker daemon not running"},
			fmt.Errorf("docker daemon is not running; start Docker manually")
	}
	return &InstallResult{Installed: true, Message: "Docker ready"}, nil
}

// EnsureDocker returns an error if Docker is missing or not running.
func EnsureDocker(ctx context.Context, log *slog.Logger) error {
	_, err := CheckAndInstall(ctx, log)
	return err
}

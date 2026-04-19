package wireguard

import (
	"context"
	"fmt"
	"log/slog"
)

// EnsureWireGuard verifies WireGuard tools are installed. CraftStack does not
// auto-install system dependencies; operators must install wireguard-tools
// manually before configuring the mesh.
func EnsureWireGuard(ctx context.Context, log *slog.Logger) error {
	mgr := NewManager(log)
	if !mgr.IsInstalled() {
		return fmt.Errorf("wireguard is not installed on this host; install wireguard-tools manually before enabling the mesh")
	}
	log.Info("WireGuard detected", "path", mgr.wgPath)
	return nil
}

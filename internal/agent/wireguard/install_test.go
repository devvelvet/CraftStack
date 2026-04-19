package wireguard

import (
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestEnsureWireGuardWithoutWG(t *testing.T) {
	t.Setenv("PATH", "")
	err := EnsureWireGuard(context.Background(), slog.Default())
	if err == nil {
		t.Fatal("expected error when wireguard is missing")
	}
	if !strings.Contains(err.Error(), "install wireguard-tools manually") {
		t.Errorf("error must instruct manual install, got: %v", err)
	}
}

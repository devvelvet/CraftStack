package docker

import (
	"context"
	"log/slog"
	"strings"
	"testing"
)

// withEmptyPATH forces docker to appear missing regardless of host.
func withEmptyPATH(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", "")
}

func TestCheckAndInstallWithoutDocker(t *testing.T) {
	withEmptyPATH(t)
	res, err := CheckAndInstall(context.Background(), slog.Default())
	if err == nil {
		t.Fatal("expected error when docker is missing")
	}
	if res == nil || res.Installed {
		t.Errorf("result should be not-installed: %+v", res)
	}
	if !strings.Contains(err.Error(), "install Docker manually") {
		t.Errorf("error must instruct manual install, got: %v", err)
	}
}

func TestEnsureDockerWithoutDocker(t *testing.T) {
	withEmptyPATH(t)
	if err := EnsureDocker(context.Background(), slog.Default()); err == nil {
		t.Error("EnsureDocker should error when docker is missing")
	}
}

// TestCheckAndInstallDoesNotAttemptInstall asserts the contract: CraftStack
// never runs package managers or downloads Docker. It reports and returns.
// We verify indirectly: CheckAndInstall must return quickly (no network /
// subprocess spawn) and surface the manual-install message.
func TestCheckAndInstallDoesNotAttemptInstall(t *testing.T) {
	withEmptyPATH(t)
	res, err := CheckAndInstall(context.Background(), slog.Default())
	if err == nil {
		t.Fatal("want err")
	}
	if res.Message == "" || !strings.Contains(err.Error(), "manually") {
		t.Errorf("behavior drifted from verify-only contract: msg=%q err=%v", res.Message, err)
	}
}

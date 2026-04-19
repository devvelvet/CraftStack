package mcoperator

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestValidateArg(t *testing.T) {
	ok := []string{"paper", "1.20.4", "--java", "21", "/usr/local/bin/x", "a.b-c_d:e=f+g"}
	bad := []string{"", "a b", "a;b", "`rm`", "$(x)", "a|b", "a&b", "a\nb"}
	for _, s := range ok {
		if err := validateArg(s); err != nil {
			t.Errorf("validateArg(%q) unexpectedly rejected: %v", s, err)
		}
	}
	for _, s := range bad {
		if err := validateArg(s); err == nil {
			t.Errorf("validateArg(%q) should have rejected", s)
		}
	}
}

func TestNewImageGenMissingBinary(t *testing.T) {
	if _, err := NewImageGen("/nonexistent/definitely-not-a-binary-xyz", "", 0); err == nil {
		t.Error("expected error for missing binary")
	}
}

func TestNewImageGenEmpty(t *testing.T) {
	g, err := NewImageGen("", "", 0)
	if err != nil || g != nil {
		t.Errorf("empty binary should return nil,nil; got %v,%v", g, err)
	}
}

// fakeBinary creates an executable shell script that records invocation args
// into a file and echoes them to stdout. Skipped on Windows.
func fakeBinary(t *testing.T, dir, logFile string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake binary not supported on windows")
	}
	path := filepath.Join(dir, "mc-imagegen-fake")
	script := "#!/bin/sh\necho \"$@\" > " + logFile + "\necho RENDERED\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake: %v", err)
	}
	return path
}

func TestImageGenRender(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "args.log")
	bin := fakeBinary(t, dir, logFile)

	g, err := NewImageGen(bin, dir, 5*time.Second)
	if err != nil {
		t.Fatalf("NewImageGen: %v", err)
	}
	res, err := g.Render(context.Background(), RenderRequest{
		Type: "paper", Version: "1.20.4", MemMB: 2048,
		ExtraArgs: []string{"--java", "21"},
	})
	if err != nil {
		t.Fatalf("Render: %v (stderr=%s)", err, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "RENDERED") {
		t.Errorf("stdout=%q", res.Stdout)
	}
	b, _ := os.ReadFile(logFile)
	args := strings.TrimSpace(string(b))
	for _, want := range []string{"render", "--type paper", "--version 1.20.4", "--memory 2048", "--java 21"} {
		if !strings.Contains(args, want) {
			t.Errorf("args missing %q: %s", want, args)
		}
	}
}

func TestImageGenRenderRejectsUnsafeArgs(t *testing.T) {
	dir := t.TempDir()
	bin := fakeBinary(t, dir, filepath.Join(dir, "x"))
	g, _ := NewImageGen(bin, dir, 5*time.Second)

	_, err := g.Render(context.Background(), RenderRequest{Type: "paper;rm -rf /", Version: "1.0"})
	if err == nil {
		t.Error("expected rejection of unsafe type")
	}
	_, err = g.Render(context.Background(), RenderRequest{Type: "paper", Version: "1.0", ExtraArgs: []string{"$(evil)"}})
	if err == nil {
		t.Error("expected rejection of unsafe extra arg")
	}
}

func TestImageGenRenderMissingFields(t *testing.T) {
	dir := t.TempDir()
	bin := fakeBinary(t, dir, filepath.Join(dir, "x"))
	g, _ := NewImageGen(bin, dir, 5*time.Second)
	if _, err := g.Render(context.Background(), RenderRequest{Type: "paper"}); err == nil {
		t.Error("expected error for missing version")
	}
	if _, err := g.Render(context.Background(), RenderRequest{Version: "1.0"}); err == nil {
		t.Error("expected error for missing type")
	}
}

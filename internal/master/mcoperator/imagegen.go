package mcoperator

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"
)

// ImageGen wraps the standalone `mc-imagegen` CLI from the mc-operator repo.
// CraftStack does not vendor the binary — the operator installs it out of band
// and points to its path via config (mc_operator.imagegen.binary).
type ImageGen struct {
	Binary    string
	OutputDir string
	Timeout   time.Duration
}

// NewImageGen validates the binary path and returns a usable wrapper, or nil
// if ImageGen is not configured.
func NewImageGen(binary, outputDir string, timeout time.Duration) (*ImageGen, error) {
	if binary == "" {
		return nil, nil
	}
	if _, err := exec.LookPath(binary); err != nil {
		return nil, fmt.Errorf("mc-imagegen binary not found: %w", err)
	}
	if outputDir == "" {
		outputDir = filepath.Join(os.TempDir(), "craftstack-imagegen")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	return &ImageGen{Binary: binary, OutputDir: outputDir, Timeout: timeout}, nil
}

// RenderRequest mirrors the mc-imagegen `render` subcommand.
type RenderRequest struct {
	Type    string // e.g. "paper", "velocity"
	Version string // e.g. "1.20.4"
	MemMB   int    // e.g. 2048
	// ExtraArgs passes through additional flags (e.g. ["--java", "21"]).
	// Values are validated against a conservative allowlist before exec.
	ExtraArgs []string
}

// RenderResult is the combined stdout/stderr and exit metadata.
type RenderResult struct {
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	OutputDir string `json:"output_dir"`
	ExitCode  int    `json:"exit_code"`
}

// safeArg permits flags and values containing only letters, digits, and a
// small set of punctuation. Rejects anything with shell metacharacters.
var safeArg = regexp.MustCompile(`^[A-Za-z0-9._:/=+-]+$`)

func validateArg(a string) error {
	if a == "" || !safeArg.MatchString(a) {
		return fmt.Errorf("unsafe argument: %q", a)
	}
	return nil
}

// Render invokes `mc-imagegen render --type T --version V --memory M ...`.
// The working directory is OutputDir so generated Dockerfile/context lands there.
func (g *ImageGen) Render(ctx context.Context, req RenderRequest) (*RenderResult, error) {
	if req.Type == "" || req.Version == "" {
		return nil, fmt.Errorf("type and version are required")
	}
	if err := validateArg(req.Type); err != nil {
		return nil, err
	}
	if err := validateArg(req.Version); err != nil {
		return nil, err
	}
	for _, a := range req.ExtraArgs {
		if err := validateArg(a); err != nil {
			return nil, err
		}
	}
	mem := req.MemMB
	if mem <= 0 {
		mem = 1024
	}

	args := []string{"render",
		"--type", req.Type,
		"--version", req.Version,
		"--memory", fmt.Sprintf("%d", mem),
	}
	args = append(args, req.ExtraArgs...)

	cctx, cancel := context.WithTimeout(ctx, g.Timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, g.Binary, args...)
	cmd.Dir = g.OutputDir
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	res := &RenderResult{
		Stdout:    outBuf.String(),
		Stderr:    errBuf.String(),
		OutputDir: g.OutputDir,
		ExitCode:  cmd.ProcessState.ExitCode(),
	}
	if err != nil {
		return res, fmt.Errorf("mc-imagegen render: %w (stderr: %s)", err, errBuf.String())
	}
	return res, nil
}

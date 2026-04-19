package agent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	pb "craftstack/gen/proto/craftstack"
)

// git is invoked via the system binary. If missing, GitCommit/GitLog return
// success=false with a descriptive message (never an error) — operators can
// still use file ops without git.

func (s *fileManagerServiceImpl) gitAvailable() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// instanceGitEnv returns the work_dir for the instance and common env vars for
// invoking git there.
func (s *fileManagerServiceImpl) instanceGitEnv(instanceID string) (string, []string, error) {
	workDir, err := s.resolveInstancePath(instanceID, "")
	if err != nil {
		return "", nil, err
	}
	if st, err := os.Stat(workDir); err != nil || !st.IsDir() {
		return "", nil, fmt.Errorf("work_dir missing for instance %s", instanceID)
	}
	// Neutral env so user-global git config doesn't leak. Force C locale so
	// the "nothing to commit" string matching works on non-English systems.
	env := append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_OPTIONAL_LOCKS=0",
		"LANG=C",
		"LC_ALL=C",
	)
	return workDir, env, nil
}

func (s *fileManagerServiceImpl) ensureRepo(ctx context.Context, workDir string, env []string) error {
	gitDir := filepath.Join(workDir, ".git")
	if st, err := os.Stat(gitDir); err == nil && st.IsDir() {
		return nil
	}
	cmd := exec.CommandContext(ctx, "git", "init", "-q", "-b", "main")
	cmd.Dir = workDir
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git init: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	// Local repo-level identity fallback (overridden per-commit anyway).
	exec.CommandContext(ctx, "git", "config", "user.name", "CraftStack").Run()
	exec.CommandContext(ctx, "git", "config", "user.email", "craftstack@localhost").Run()
	return nil
}

// GitCommit stages and commits changes in the instance work_dir.
func (s *fileManagerServiceImpl) GitCommit(ctx context.Context, req *pb.GitCommitRequest) (*pb.GitCommitResponse, error) {
	if !s.gitAvailable() {
		return &pb.GitCommitResponse{Success: false, Message: "git not installed on this host"}, nil
	}
	workDir, env, err := s.instanceGitEnv(req.InstanceId)
	if err != nil {
		return nil, err
	}
	if err := s.ensureRepo(ctx, workDir, env); err != nil {
		return &pb.GitCommitResponse{Success: false, Message: err.Error()}, nil
	}

	// Stage.
	addArgs := []string{"add", "--"}
	if len(req.Paths) == 0 {
		addArgs = []string{"add", "-A"}
	} else {
		for _, p := range req.Paths {
			// Validate each path is within work_dir.
			if _, err := s.resolveInstancePath(req.InstanceId, p); err != nil {
				return &pb.GitCommitResponse{Success: false, Message: "invalid path: " + p}, nil
			}
			addArgs = append(addArgs, p)
		}
	}
	addCmd := exec.CommandContext(ctx, "git", addArgs...)
	addCmd.Dir = workDir
	addCmd.Env = env
	if out, err := addCmd.CombinedOutput(); err != nil {
		return &pb.GitCommitResponse{Success: false, Message: "git add: " + strings.TrimSpace(string(out))}, nil
	}

	author := strings.TrimSpace(req.AuthorName)
	if author == "" {
		author = "CraftStack"
	}
	email := strings.TrimSpace(req.AuthorEmail)
	if email == "" {
		email = "craftstack@localhost"
	}
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		msg = "change"
	}

	commitArgs := []string{"commit", "-m", msg}
	if req.AllowEmpty {
		commitArgs = append(commitArgs, "--allow-empty")
	}
	commitCmd := exec.CommandContext(ctx, "git", commitArgs...)
	commitCmd.Dir = workDir
	commitCmd.Env = append(env,
		"GIT_AUTHOR_NAME="+author,
		"GIT_AUTHOR_EMAIL="+email,
		"GIT_COMMITTER_NAME="+author,
		"GIT_COMMITTER_EMAIL="+email,
	)
	var stdout, stderr bytes.Buffer
	commitCmd.Stdout = &stdout
	commitCmd.Stderr = &stderr
	if err := commitCmd.Run(); err != nil {
		// "nothing to commit" is a benign case.
		combined := stdout.String() + stderr.String()
		if strings.Contains(combined, "nothing to commit") || strings.Contains(combined, "nothing added") {
			return &pb.GitCommitResponse{Success: true, Message: "no changes"}, nil
		}
		return &pb.GitCommitResponse{Success: false, Message: "git commit: " + strings.TrimSpace(combined)}, nil
	}

	// Capture short SHA.
	shaCmd := exec.CommandContext(ctx, "git", "rev-parse", "--short", "HEAD")
	shaCmd.Dir = workDir
	shaCmd.Env = env
	shaOut, _ := shaCmd.Output()
	return &pb.GitCommitResponse{
		Success:   true,
		Message:   "committed",
		CommitSha: strings.TrimSpace(string(shaOut)),
	}, nil
}

// GitRestore restores a single file to the content it had at a given commit.
// It uses `git show <sha>:<path>` to fetch the content (works even if the file
// was later deleted), writes it back, and creates a rollback commit.
func (s *fileManagerServiceImpl) GitRestore(ctx context.Context, req *pb.GitRestoreRequest) (*pb.GitRestoreResponse, error) {
	if !s.gitAvailable() {
		return &pb.GitRestoreResponse{Success: false, Message: "git not installed"}, nil
	}
	if strings.TrimSpace(req.Path) == "" || strings.TrimSpace(req.CommitSha) == "" {
		return nil, fmt.Errorf("path and commit_sha are required")
	}
	targetAbs, err := s.resolveInstancePath(req.InstanceId, req.Path)
	if err != nil {
		return nil, err
	}
	workDir, env, err := s.instanceGitEnv(req.InstanceId)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(workDir, ".git")); err != nil {
		return &pb.GitRestoreResponse{Success: false, Message: "repo not initialized"}, nil
	}

	// Validate SHA — must be hex and resolve to an existing commit in this repo.
	for _, c := range req.CommitSha {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return nil, fmt.Errorf("invalid commit sha")
		}
	}
	verify := exec.CommandContext(ctx, "git", "cat-file", "-e", req.CommitSha+"^{commit}")
	verify.Dir = workDir
	verify.Env = env
	if out, err := verify.CombinedOutput(); err != nil {
		return &pb.GitRestoreResponse{Success: false, Message: "unknown commit: " + strings.TrimSpace(string(out))}, nil
	}

	// Fetch the file content at that commit.
	show := exec.CommandContext(ctx, "git", "show", req.CommitSha+":"+req.Path)
	show.Dir = workDir
	show.Env = env
	content, err := show.Output()
	if err != nil {
		return &pb.GitRestoreResponse{Success: false, Message: "file not present in that commit"}, nil
	}

	// Ensure parent dir and overwrite the file.
	if err := os.MkdirAll(filepath.Dir(targetAbs), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir parent: %w", err)
	}
	if err := os.WriteFile(targetAbs, content, 0o644); err != nil {
		return nil, fmt.Errorf("write restored content: %w", err)
	}

	// Commit the rollback.
	commitResp, err := s.GitCommit(ctx, &pb.GitCommitRequest{
		InstanceId: req.InstanceId,
		Paths:      []string{req.Path},
		Message:    fmt.Sprintf("rollback %s to %s", req.Path, req.CommitSha),
		AuthorName: req.AuthorName, AuthorEmail: req.AuthorEmail,
	})
	if err != nil {
		return nil, err
	}
	s.log.Info("file restored", "instance", req.InstanceId, "path", req.Path, "from", req.CommitSha, "new_sha", commitResp.CommitSha)
	return &pb.GitRestoreResponse{
		Success:   commitResp.Success,
		Message:   commitResp.Message,
		CommitSha: commitResp.CommitSha,
	}, nil
}

// GitLog returns commits touching a path. Uses `--` to avoid ambiguity.
func (s *fileManagerServiceImpl) GitLog(ctx context.Context, req *pb.GitLogRequest) (*pb.GitLogResponse, error) {
	if !s.gitAvailable() {
		return &pb.GitLogResponse{Success: false, Message: "git not installed"}, nil
	}
	workDir, env, err := s.instanceGitEnv(req.InstanceId)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(workDir, ".git")); err != nil {
		return &pb.GitLogResponse{Success: false, Message: "no git history (repo not initialized)"}, nil
	}
	limit := int(req.Limit)
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	// --- format: SHA|author|email|unixtime|subject (NUL-separated to survive pipes in msg)
	args := []string{"log", "-n", strconv.Itoa(limit),
		"--pretty=format:%h%x00%an%x00%ae%x00%at%x00%s"}
	if p := strings.TrimSpace(req.Path); p != "" {
		if _, err := s.resolveInstancePath(req.InstanceId, p); err != nil {
			return &pb.GitLogResponse{Success: false, Message: "invalid path"}, nil
		}
		args = append(args, "--", p)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = workDir
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		return &pb.GitLogResponse{Success: false, Message: "git log: " + err.Error()}, nil
	}
	var commits []*pb.GitCommitEntry
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\x00", 5)
		if len(parts) < 5 {
			continue
		}
		ts, _ := strconv.ParseInt(parts[3], 10, 64)
		commits = append(commits, &pb.GitCommitEntry{
			Sha:           parts[0],
			Author:        parts[1],
			Email:         parts[2],
			TimestampUnix: ts,
			Message:       parts[4],
		})
	}
	return &pb.GitLogResponse{Success: true, Commits: commits}, nil
}

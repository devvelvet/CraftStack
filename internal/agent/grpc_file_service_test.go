package agent

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	pb "craftstack/gen/proto/craftstack"
	"craftstack/internal/common"
)

// newTestFileService builds a fileManagerServiceImpl with a single instance
// whose work_dir is the given temp directory. No goroutines, no gRPC.
func newTestFileService(t *testing.T, workDir string) (*fileManagerServiceImpl, string) {
	t.Helper()
	a := &Agent{
		mu:   sync.RWMutex{},
		defs: map[string]*common.InstanceDef{},
	}
	inst := "inst-test"
	a.defs[inst] = &common.InstanceDef{Name: inst, WorkDir: workDir}
	return &fileManagerServiceImpl{agent: a, log: slog.Default()}, inst
}

func writeFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// ── CopyFile ──

func TestCopyFile_FileToNewFile(t *testing.T) {
	dir := t.TempDir()
	s, inst := newTestFileService(t, dir)
	writeFile(t, filepath.Join(dir, "a.txt"), "hello")

	resp, err := s.CopyFile(context.Background(), &pb.CopyFileRequest{
		InstanceId: inst, SrcPath: "a.txt", DstPath: "b.txt",
	})
	if err != nil || !resp.Success {
		t.Fatalf("copy: err=%v resp=%+v", err, resp)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "b.txt"))
	if string(got) != "hello" {
		t.Errorf("content=%q", got)
	}
	// src still there
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); err != nil {
		t.Error("source missing after copy")
	}
}

func TestCopyFile_DirectoryRecursive(t *testing.T) {
	dir := t.TempDir()
	s, inst := newTestFileService(t, dir)
	writeFile(t, filepath.Join(dir, "src/a/1.txt"), "one")
	writeFile(t, filepath.Join(dir, "src/a/b/2.txt"), "two")

	resp, err := s.CopyFile(context.Background(), &pb.CopyFileRequest{
		InstanceId: inst, SrcPath: "src", DstPath: "dst",
	})
	if err != nil || !resp.Success {
		t.Fatalf("copy dir: err=%v resp=%+v", err, resp)
	}
	c1, _ := os.ReadFile(filepath.Join(dir, "dst/a/1.txt"))
	c2, _ := os.ReadFile(filepath.Join(dir, "dst/a/b/2.txt"))
	if string(c1) != "one" || string(c2) != "two" {
		t.Errorf("recursive copy content wrong: %q %q", c1, c2)
	}
}

func TestCopyFile_RejectExistingWithoutOverwrite(t *testing.T) {
	dir := t.TempDir()
	s, inst := newTestFileService(t, dir)
	writeFile(t, filepath.Join(dir, "a.txt"), "hello")
	writeFile(t, filepath.Join(dir, "b.txt"), "world")

	_, err := s.CopyFile(context.Background(), &pb.CopyFileRequest{
		InstanceId: inst, SrcPath: "a.txt", DstPath: "b.txt",
	})
	if err == nil {
		t.Fatal("expected error when destination exists and overwrite=false")
	}
	got, _ := os.ReadFile(filepath.Join(dir, "b.txt"))
	if string(got) != "world" {
		t.Errorf("dst should be unchanged, got %q", got)
	}
}

func TestCopyFile_AllowsOverwrite(t *testing.T) {
	dir := t.TempDir()
	s, inst := newTestFileService(t, dir)
	writeFile(t, filepath.Join(dir, "a.txt"), "NEW")
	writeFile(t, filepath.Join(dir, "b.txt"), "OLD")

	resp, err := s.CopyFile(context.Background(), &pb.CopyFileRequest{
		InstanceId: inst, SrcPath: "a.txt", DstPath: "b.txt", Overwrite: true,
	})
	if err != nil || !resp.Success {
		t.Fatalf("overwrite copy: err=%v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "b.txt"))
	if string(got) != "NEW" {
		t.Errorf("overwrite failed: %q", got)
	}
}

func TestCopyFile_RejectSelfCopy(t *testing.T) {
	dir := t.TempDir()
	s, inst := newTestFileService(t, dir)
	writeFile(t, filepath.Join(dir, "a.txt"), "hi")
	_, err := s.CopyFile(context.Background(), &pb.CopyFileRequest{
		InstanceId: inst, SrcPath: "a.txt", DstPath: "a.txt",
	})
	if err == nil {
		t.Error("self-copy should error")
	}
}

func TestCopyFile_RejectCopyDirIntoItself(t *testing.T) {
	dir := t.TempDir()
	s, inst := newTestFileService(t, dir)
	writeFile(t, filepath.Join(dir, "src/a.txt"), "x")
	_, err := s.CopyFile(context.Background(), &pb.CopyFileRequest{
		InstanceId: inst, SrcPath: "src", DstPath: "src/inner",
	})
	if err == nil {
		t.Error("copy dir into self should error")
	}
}

func TestCopyFile_PathTraversalRejected(t *testing.T) {
	dir := t.TempDir()
	s, inst := newTestFileService(t, dir)
	writeFile(t, filepath.Join(dir, "a.txt"), "x")
	_, err := s.CopyFile(context.Background(), &pb.CopyFileRequest{
		InstanceId: inst, SrcPath: "a.txt", DstPath: "../escape.txt",
	})
	if err == nil {
		t.Error("traversal dst should be rejected")
	}
}

// ── Rename (exercised via existing code path, sanity check) ──

func TestRenameFile_Move(t *testing.T) {
	dir := t.TempDir()
	s, inst := newTestFileService(t, dir)
	writeFile(t, filepath.Join(dir, "a.txt"), "x")
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	resp, err := s.RenameFile(context.Background(), &pb.RenameFileRequest{
		InstanceId: inst, OldPath: "a.txt", NewPath: "sub/a.txt",
	})
	if err != nil || !resp.Success {
		t.Fatalf("rename: err=%v resp=%+v", err, resp)
	}
	if _, err := os.Stat(filepath.Join(dir, "sub/a.txt")); err != nil {
		t.Error("dst missing")
	}
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); err == nil {
		t.Error("src should be gone")
	}
}

// ── Git ──

func TestGitCommit_InitsAndCommits(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	s, inst := newTestFileService(t, dir)
	writeFile(t, filepath.Join(dir, "hello.txt"), "hi")

	resp, err := s.GitCommit(context.Background(), &pb.GitCommitRequest{
		InstanceId: inst, Paths: []string{"hello.txt"},
		Message:    "first commit",
		AuthorName: "alice", AuthorEmail: "alice@example.com",
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if !resp.Success {
		t.Fatalf("commit not successful: %q", resp.Message)
	}
	if resp.CommitSha == "" {
		t.Error("empty sha")
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Error("repo not initialized")
	}
}

func TestGitCommit_NoChangesIsBenign(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	s, inst := newTestFileService(t, dir)
	writeFile(t, filepath.Join(dir, "a.txt"), "x")
	// first commit
	_, _ = s.GitCommit(context.Background(), &pb.GitCommitRequest{
		InstanceId: inst, Paths: []string{"a.txt"},
		Message: "init", AuthorName: "a", AuthorEmail: "a@x",
	})
	// second with no changes → success=true, message=no changes
	resp, err := s.GitCommit(context.Background(), &pb.GitCommitRequest{
		InstanceId: inst, Paths: []string{"a.txt"},
		Message: "again", AuthorName: "a", AuthorEmail: "a@x",
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !resp.Success {
		t.Errorf("no-op should be success=true, got %+v", resp)
	}
}

func TestGitLog_ReturnsAuthorAndMessage(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	s, inst := newTestFileService(t, dir)
	writeFile(t, filepath.Join(dir, "x.txt"), "1")

	if _, err := s.GitCommit(context.Background(), &pb.GitCommitRequest{
		InstanceId: inst, Paths: []string{"x.txt"},
		Message: "c1", AuthorName: "bob", AuthorEmail: "bob@x",
	}); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "x.txt"), "2")
	if _, err := s.GitCommit(context.Background(), &pb.GitCommitRequest{
		InstanceId: inst, Paths: []string{"x.txt"},
		Message: "c2 updated", AuthorName: "carol", AuthorEmail: "carol@x",
	}); err != nil {
		t.Fatal(err)
	}

	resp, err := s.GitLog(context.Background(), &pb.GitLogRequest{
		InstanceId: inst, Path: "x.txt", Limit: 10,
	})
	if err != nil {
		t.Fatalf("log err: %v", err)
	}
	if !resp.Success {
		t.Fatalf("log not success: %q", resp.Message)
	}
	if len(resp.Commits) != 2 {
		t.Fatalf("want 2 commits, got %d: %+v", len(resp.Commits), resp.Commits)
	}
	// newest first
	if resp.Commits[0].Author != "carol" || resp.Commits[0].Message != "c2 updated" {
		t.Errorf("head commit wrong: %+v", resp.Commits[0])
	}
	if resp.Commits[1].Author != "bob" {
		t.Errorf("prev commit author: %+v", resp.Commits[1])
	}
	if resp.Commits[0].TimestampUnix <= 0 {
		t.Error("timestamp missing")
	}
}

func TestGitLog_NoRepoYetIsFriendly(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	s, inst := newTestFileService(t, dir)
	resp, err := s.GitLog(context.Background(), &pb.GitLogRequest{InstanceId: inst, Path: ""})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp.Success {
		t.Error("no repo → success should be false")
	}
}

// ── GitRestore ──

func TestGitRestore_RevertsFileContent(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	s, inst := newTestFileService(t, dir)

	writeFile(t, filepath.Join(dir, "conf.yml"), "v1")
	c1, err := s.GitCommit(context.Background(), &pb.GitCommitRequest{
		InstanceId: inst, Paths: []string{"conf.yml"},
		Message: "v1", AuthorName: "a", AuthorEmail: "a@x",
	})
	if err != nil || !c1.Success {
		t.Fatalf("c1: %v %+v", err, c1)
	}
	firstSHA := c1.CommitSha

	writeFile(t, filepath.Join(dir, "conf.yml"), "v2-broken")
	c2, err := s.GitCommit(context.Background(), &pb.GitCommitRequest{
		InstanceId: inst, Paths: []string{"conf.yml"},
		Message: "v2", AuthorName: "a", AuthorEmail: "a@x",
	})
	if err != nil || !c2.Success {
		t.Fatalf("c2: %v", err)
	}

	// Restore to v1.
	r, err := s.GitRestore(context.Background(), &pb.GitRestoreRequest{
		InstanceId: inst, Path: "conf.yml", CommitSha: firstSHA,
		AuthorName: "alice", AuthorEmail: "alice@x",
	})
	if err != nil || !r.Success {
		t.Fatalf("restore: err=%v resp=%+v", err, r)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "conf.yml"))
	if string(got) != "v1" {
		t.Errorf("content=%q want v1", got)
	}
	if r.CommitSha == "" || r.CommitSha == firstSHA {
		t.Errorf("expected new rollback sha, got %q", r.CommitSha)
	}

	// Log should now show a rollback commit authored by alice.
	lg, _ := s.GitLog(context.Background(), &pb.GitLogRequest{InstanceId: inst, Path: "conf.yml"})
	if len(lg.Commits) != 3 {
		t.Errorf("want 3 commits, got %d", len(lg.Commits))
	}
	if lg.Commits[0].Author != "alice" {
		t.Errorf("rollback author: %q", lg.Commits[0].Author)
	}
	if !containsAll(lg.Commits[0].Message, "rollback", firstSHA) {
		t.Errorf("rollback message: %q", lg.Commits[0].Message)
	}
}

func TestGitRestore_RejectsInvalidSHA(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	s, inst := newTestFileService(t, dir)
	writeFile(t, filepath.Join(dir, "a"), "x")
	if _, err := s.GitCommit(context.Background(), &pb.GitCommitRequest{
		InstanceId: inst, Paths: []string{"a"}, Message: "x", AuthorName: "a", AuthorEmail: "a@x",
	}); err != nil {
		t.Fatal(err)
	}
	// non-hex
	if _, err := s.GitRestore(context.Background(), &pb.GitRestoreRequest{
		InstanceId: inst, Path: "a", CommitSha: "not-a-sha;rm",
	}); err == nil {
		t.Error("should reject non-hex sha")
	}
	// unknown sha (hex but nonexistent)
	r, err := s.GitRestore(context.Background(), &pb.GitRestoreRequest{
		InstanceId: inst, Path: "a", CommitSha: "deadbeefdeadbeef",
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if r.Success {
		t.Error("unknown sha should not succeed")
	}
}

func TestGitRestore_RequiresPathAndSHA(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	s, inst := newTestFileService(t, dir)
	if _, err := s.GitRestore(context.Background(), &pb.GitRestoreRequest{
		InstanceId: inst, Path: "", CommitSha: "abc123",
	}); err == nil {
		t.Error("empty path should error")
	}
	if _, err := s.GitRestore(context.Background(), &pb.GitRestoreRequest{
		InstanceId: inst, Path: "a", CommitSha: "",
	}); err == nil {
		t.Error("empty sha should error")
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !stringsContains(s, sub) {
			return false
		}
	}
	return true
}
func stringsContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestGitCommit_NoGitBinaryIsGraceful(t *testing.T) {
	// Force gitAvailable() to return false via empty PATH.
	t.Setenv("PATH", "")
	dir := t.TempDir()
	s, inst := newTestFileService(t, dir)
	resp, err := s.GitCommit(context.Background(), &pb.GitCommitRequest{
		InstanceId: inst, Message: "x", AuthorName: "a", AuthorEmail: "a@x",
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp.Success {
		t.Error("should be success=false when git missing")
	}
}

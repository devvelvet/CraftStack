package web

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	pb "craftstack/gen/proto/craftstack"
	"craftstack/internal/master/store"
)

// auditFileAction records who did what to which file. Silent on DB errors so
// it never blocks the user-facing operation; the webserver log carries failures.
func (s *Server) auditFileAction(c echo.Context, action, instanceID, path, detail string) {
	userID, username, _ := getCurrentUser(c)
	var uid *int
	if userID > 0 {
		uid = &userID
	}
	if username == "" {
		username = "system"
	}
	e := &store.AuditLog{
		UserID:     uid,
		Username:   username,
		Action:     action,
		TargetType: "file",
		TargetID:   instanceID + ":" + path,
		TargetName: path,
		Detail:     detail,
	}
	if err := s.db.CreateAuditLog(e); err != nil {
		s.log.Warn("audit log insert failed", "error", err, "action", action, "path", path)
	}
}

// gitCommitFiles commits paths inside an instance's work_dir on behalf of the
// current user. Best-effort: any failure is logged but not returned to the
// caller, so file-op success is not gated on git availability.
func (s *Server) gitCommitFiles(ctx context.Context, c echo.Context, instanceID string, paths []string, verb string) {
	userID, username, _ := getCurrentUser(c)
	email := fmt.Sprintf("user-%d@craftstack.local", userID)
	if u, err := s.db.GetUserByUsername(username); err == nil && u != nil {
		// No email column today; derive a stable one from the username.
		email = fmt.Sprintf("%s@craftstack.local", strings.ReplaceAll(u.Username, " ", "-"))
	}
	msg := fmt.Sprintf("[%s] %s: %s", username, verb, strings.Join(paths, ", "))
	if len(msg) > 200 {
		msg = msg[:200]
	}

	client, conn, err := s.connectFileManager(instanceID)
	if err != nil {
		s.log.Warn("git commit skipped (no agent conn)", "instance", instanceID, "error", err)
		return
	}
	defer conn.Close()

	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	resp, err := client.GitCommit(cctx, &pb.GitCommitRequest{
		InstanceId:  instanceID,
		Paths:       paths,
		Message:     msg,
		AuthorName:  username,
		AuthorEmail: email,
	})
	if err != nil {
		s.log.Warn("git commit rpc failed", "instance", instanceID, "error", err)
		return
	}
	if !resp.Success {
		s.log.Info("git commit not applied", "instance", instanceID, "reason", resp.Message)
		return
	}
	s.log.Info("git commit", "instance", instanceID, "sha", resp.CommitSha, "paths", paths)
}

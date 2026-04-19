package web

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/labstack/echo/v4"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"craftstack/internal/master/store"

	pb "craftstack/gen/proto/craftstack"
)

// connectFileManager creates a gRPC connection to the agent's FileManagerService for a given instance.
func (s *Server) connectFileManager(instanceID string) (pb.FileManagerServiceClient, *grpc.ClientConn, error) {
	agentID, found := s.connector.GetInstanceOwner(instanceID)
	if !found {
		return nil, nil, fmt.Errorf("the instance register did not")
	}

	agentAddr, ok := s.connector.GetAgentAddress(agentID)
	if !ok {
		return nil, nil, fmt.Errorf("the agent offline")
	}

	conn, err := grpc.NewClient(agentAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(256*1024*1024), // 256MB receive
			grpc.MaxCallSendMsgSize(256*1024*1024), // 256MB send
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("agent connection failed: %v", err)
	}

	return pb.NewFileManagerServiceClient(conn), conn, nil
}

// handleFileManager renders the file manager page.
func (s *Server) handleFileManager(c echo.Context) error {
	id := c.Param("id")
	inst, err := s.db.GetInstance(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "instance not found")
	}
	s.overlayInstanceStatus([]*store.Instance{inst})
	data := map[string]interface{}{
		"Title":    fmt.Sprintf("file manage - %s", inst.Name),
		"Instance": inst,
	}
	return renderPage(c, "file_manager", data)
}

// apiListFiles returns directory listing for an instance.
func (s *Server) apiListFiles(c echo.Context) error {
	id := c.Param("id")
	path := c.QueryParam("path")

	client, conn, err := s.connectFileManager(id)
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(c.Request().Context(), 10*time.Second)
	defer cancel()

	resp, err := client.ListDirectory(ctx, &pb.ListDirectoryRequest{
		InstanceId: id,
		Path:       path,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, resp)
}

// apiReadFile returns file content.
func (s *Server) apiReadFile(c echo.Context) error {
	id := c.Param("id")
	path := c.QueryParam("path")

	client, conn, err := s.connectFileManager(id)
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(c.Request().Context(), 30*time.Second)
	defer cancel()

	resp, err := client.ReadFile(ctx, &pb.ReadFileRequest{
		InstanceId: id,
		Path:       path,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"path":      resp.Path,
		"content":   base64.StdEncoding.EncodeToString(resp.Content),
		"size":      resp.Size,
		"truncated": resp.Truncated,
	})
}

// apiWriteFile writes content to a file.
func (s *Server) apiWriteFile(c echo.Context) error {
	id := c.Param("id")

	var req struct {
		Path       string `json:"path"`
		Content    string `json:"content"` // base64 encoded
		CreateDirs bool   `json:"create_dirs"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	content, err := base64.StdEncoding.DecodeString(req.Content)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid base64 inload"})
	}

	client, conn, err := s.connectFileManager(id)
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(c.Request().Context(), 30*time.Second)
	defer cancel()

	resp, err := client.WriteFile(ctx, &pb.WriteFileRequest{
		InstanceId: id,
		Path:       req.Path,
		Content:    content,
		CreateDirs: req.CreateDirs,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, resp)
}

// apiDeleteFile deletes a file or directory.
func (s *Server) apiDeleteFile(c echo.Context) error {
	id := c.Param("id")

	var req struct {
		Path      string `json:"path"`
		Recursive bool   `json:"recursive"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	client, conn, err := s.connectFileManager(id)
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(c.Request().Context(), 10*time.Second)
	defer cancel()

	resp, err := client.DeleteFile(ctx, &pb.DeleteFileRequest{
		InstanceId: id,
		Path:       req.Path,
		Recursive:  req.Recursive,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, resp)
}

// apiCreateDir creates a directory.
func (s *Server) apiCreateDir(c echo.Context) error {
	id := c.Param("id")

	var req struct {
		Path string `json:"path"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	client, conn, err := s.connectFileManager(id)
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(c.Request().Context(), 10*time.Second)
	defer cancel()

	resp, err := client.CreateDirectory(ctx, &pb.CreateDirectoryRequest{
		InstanceId: id,
		Path:       req.Path,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, resp)
}

// apiRenameFile renames a file or directory.
func (s *Server) apiRenameFile(c echo.Context) error {
	id := c.Param("id")

	var req struct {
		OldPath string `json:"old_path"`
		NewPath string `json:"new_path"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	client, conn, err := s.connectFileManager(id)
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(c.Request().Context(), 10*time.Second)
	defer cancel()

	resp, err := client.RenameFile(ctx, &pb.RenameFileRequest{
		InstanceId: id,
		OldPath:    req.OldPath,
		NewPath:    req.NewPath,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, resp)
}

// apiUploadFile handles file upload via multipart form.
func (s *Server) apiUploadFile(c echo.Context) error {
	id := c.Param("id")
	path := c.FormValue("path")

	file, err := c.FormFile("file")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "file is missing"})
	}

	src, err := file.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "file open failed"})
	}
	defer src.Close()

	content, err := io.ReadAll(src)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "file read failed"})
	}

	// path directory if filename add
	uploadPath := path
	if uploadPath == "" || uploadPath == "." {
		uploadPath = file.Filename
	} else {
		uploadPath = uploadPath + "/" + file.Filename
	}

	client, conn, err := s.connectFileManager(id)
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Minute)
	defer cancel()

	resp, err := client.WriteFile(ctx, &pb.WriteFileRequest{
		InstanceId: id,
		Path:       uploadPath,
		Content:    content,
		CreateDirs: true,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, resp)
}

// apiDownloadFile downloads a file from the agent (streaming response).
func (s *Server) apiDownloadFile(c echo.Context) error {
	id := c.Param("id")
	path := c.QueryParam("path")

	client, conn, err := s.connectFileManager(id)
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Minute)
	defer cancel()

	resp, err := client.ReadFile(ctx, &pb.ReadFileRequest{
		InstanceId: id,
		Path:       path,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// Extract filename from path
	filename := path
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			filename = path[i+1:]
			break
		}
	}

	// URL-encode filename for non-ASCII characters (RFC 5987)
	c.Response().Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, filename, url.PathEscape(filename)))
	c.Response().Header().Set("Content-Type", "application/octet-stream")
	c.Response().Header().Set("Content-Length", fmt.Sprintf("%d", len(resp.Content)))
	c.Response().WriteHeader(http.StatusOK)

	// Write in chunks to avoid holding full buffer in HTTP layer
	buf := resp.Content
	for len(buf) > 0 {
		n := len(buf)
		if n > 64*1024 {
			n = 64 * 1024 // 64KB chunks
		}
		if _, err := c.Response().Write(buf[:n]); err != nil {
			return err
		}
		c.Response().Flush()
		buf = buf[n:]
	}
	return nil
}

package web

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "craftstack/gen/proto/craftstack"
)

// handleDatabaseBrowser renders the database browser page for DB instances.
func (s *Server) handleDatabaseBrowser(c echo.Context) error {
	id := c.Param("id")
	inst, err := s.db.GetInstance(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "instance not found")
	}

	// DB typeonly allow
	switch inst.InstanceType {
	case "mysql", "postgresql", "mongodb", "redis":
		// OK
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "database type instanceonly support")
	}

	// node offlineif instance offline
	if !s.connector.IsAgentOnline(inst.NodeID) {
		inst.Status = "offline"
	}

	data := map[string]interface{}{
		"Title":    fmt.Sprintf("database: %s", inst.Name),
		"Instance": inst,
	}
	return renderPage(c, "database_browser", data)
}

// apiExecuteQuery executes a query/command against a DB instance via gRPC SendCommand.
// POST /api/instances/:id/query
// Request: { "query": "SELECT * FROM users" }
// Response: { "success": true, "output": "..." } or { "success": false, "error": "..." }
func (s *Server) apiExecuteQuery(c echo.Context) error {
	id := c.Param("id")
	var req struct {
		Query string `json:"query"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "invalid request",
		})
	}

	if strings.TrimSpace(req.Query) == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "query is empty",
		})
	}

	// instance query
	inst, err := s.db.GetInstance(id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]interface{}{
			"success": false,
			"error":   "instance not found",
		})
	}

	// agent mapping query
	agentID, found := s.connector.GetInstanceOwner(id)
	if !found {
		return c.JSON(http.StatusNotFound, map[string]interface{}{
			"success": false,
			"error":   "the instance register did not",
		})
	}

	agentAddr, ok := s.connector.GetAgentAddress(agentID)
	if !ok {
		return c.JSON(http.StatusServiceUnavailable, map[string]interface{}{
			"success": false,
			"error":   "the agent offline",
		})
	}

	// agent gRPC call — SendCommand
	conn, err := grpc.NewClient(agentAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("agent connection failed: %v", err),
		})
	}
	defer conn.Close()

	// DB query time long may take time, so 30s timeout
	ctx, cancel := context.WithTimeout(c.Request().Context(), 30*time.Second)
	defer cancel()

	metricsClient := pb.NewMetricsServiceClient(conn)
	resp, err := metricsClient.SendCommand(ctx, &pb.ConsoleCommandRequest{
		AgentId:    agentID,
		InstanceId: id,
		Command:    req.Query,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("command execution failed: %v", err),
		})
	}

	if !resp.Success {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   resp.Error,
		})
	}

	// audit log
	s.audit(c, "query", "instance", id, inst.Name, "", "", "",
		fmt.Sprintf("DB query execute: %s", truncateStr(req.Query, 100)))

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"output":  resp.Output,
	})
}

// truncateStr truncates a string to maxLen, appending "..." if truncated.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

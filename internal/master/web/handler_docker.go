package web

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "craftstack/gen/proto/craftstack"
)

// apiCheckDocker checks if Docker is installed and running on a specific agent.
func (s *Server) apiCheckDocker(c echo.Context) error {
	nodeID := c.Param("id")

	agentAddr, ok := s.connector.GetAgentAddress(nodeID)
	if !ok {
		return c.JSON(http.StatusServiceUnavailable, map[string]interface{}{
			"installed": false,
			"running":   false,
			"message":   "the agent offline",
		})
	}

	conn, err := grpc.NewClient(agentAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("agent connection failed: %v", err),
		})
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(c.Request().Context(), 10*time.Second)
	defer cancel()

	agentClient := pb.NewAgentServiceClient(conn)
	resp, err := agentClient.CheckDocker(ctx, &pb.CheckDockerRequest{})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Docker state check failed: %v", err),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"installed": resp.Installed,
		"running":   resp.Running,
		"version":   resp.Version,
		"message":   resp.Message,
	})
}

// apiInstallDocker installs Docker on a specific agent.
func (s *Server) apiInstallDocker(c echo.Context) error {
	nodeID := c.Param("id")

	agentAddr, ok := s.connector.GetAgentAddress(nodeID)
	if !ok {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"status":  "error",
			"message": "the agent offline",
		})
	}

	conn, err := grpc.NewClient(agentAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"status":  "error",
			"message": fmt.Sprintf("agent connection failed: %v", err),
		})
	}
	defer conn.Close()

	// Docker install may take time, so use a longer timeout
	ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Minute)
	defer cancel()

	agentClient := pb.NewAgentServiceClient(conn)
	resp, err := agentClient.InstallDocker(ctx, &pb.InstallDockerRequest{})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"status":  "error",
			"message": fmt.Sprintf("Docker install failed: %v", err),
		})
	}

	if !resp.Success {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"status":  "error",
			"message": resp.Message,
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":  "success",
		"version": resp.Version,
		"message": resp.Message,
	})
}

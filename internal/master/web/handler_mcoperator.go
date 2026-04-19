package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"craftstack/internal/master/mcoperator"
)

// handleJenkinsForward accepts a Jenkins webhook and forwards it to mc-operator.
// The inbound request must present the shared token (Bearer or ?token=).
// Auth on the outbound call uses the mc-operator Bearer token from config.
func (s *Server) handleJenkinsForward(c echo.Context) error {
	shared := s.mcop.JenkinsSharedToken
	if shared != "" {
		got := strings.TrimPrefix(c.Request().Header.Get("Authorization"), "Bearer ")
		if got == "" {
			got = c.QueryParam("token")
		}
		if got != shared {
			return echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
		}
	}

	var t mcoperator.JenkinsTrigger
	if err := json.NewDecoder(c.Request().Body).Decode(&t); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid json: "+err.Error())
	}
	if t.Server == "" || t.Image == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "server and image are required")
	}

	if err := s.mcop.Client.TriggerJenkins(c.Request().Context(), t); err != nil {
		s.log.Warn("mc-operator jenkins forward failed", "error", err, "server", t.Server)
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	s.log.Info("jenkins webhook forwarded to mc-operator", "server", t.Server, "image", t.Image, "buildId", t.BuildID)
	return c.JSON(http.StatusAccepted, map[string]string{"status": "forwarded", "server": t.Server})
}

// apiMCOperatorSync triggers mc-operator's JAR pipeline for a named server.
func (s *Server) apiMCOperatorSync(c echo.Context) error {
	server := c.Param("server")
	if server == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "server name required")
	}
	if err := s.mcop.Client.Sync(c.Request().Context(), server); err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	return c.JSON(http.StatusAccepted, map[string]string{"status": "synced", "server": server})
}

// apiMCOperatorServers proxies mc-operator's /api/v1/servers snapshot.
func (s *Server) apiMCOperatorServers(c echo.Context) error {
	raw, err := s.mcop.Client.Servers(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	return c.Blob(http.StatusOK, "application/json", raw)
}

// apiMCOperatorImageGen invokes the mc-imagegen CLI to render a server image.
// Request body:
//
//	{ "type": "paper", "version": "1.20.4", "memory_mb": 2048, "extra_args": ["--java","21"] }
func (s *Server) apiMCOperatorImageGen(c echo.Context) error {
	var req struct {
		Type      string   `json:"type"`
		Version   string   `json:"version"`
		MemoryMB  int      `json:"memory_mb"`
		ExtraArgs []string `json:"extra_args"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid json: "+err.Error())
	}
	res, err := s.mcop.ImageGen.Render(c.Request().Context(), mcoperator.RenderRequest{
		Type:      req.Type,
		Version:   req.Version,
		MemMB:     req.MemoryMB,
		ExtraArgs: req.ExtraArgs,
	})
	if err != nil {
		status := http.StatusInternalServerError
		if res != nil && res.ExitCode != 0 {
			status = http.StatusBadRequest
		}
		s.log.Warn("mc-imagegen render failed", "error", err, "type", req.Type, "version", req.Version)
		return c.JSON(status, map[string]any{"error": err.Error(), "result": res})
	}
	s.log.Info("mc-imagegen render ok", "type", req.Type, "version", req.Version, "out", res.OutputDir)
	return c.JSON(http.StatusOK, res)
}

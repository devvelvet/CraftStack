package web

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

func (s *Server) apiListNodes(c echo.Context) error {
	nodes, err := s.db.ListNodes()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	s.overlayNodeStatus(nodes)
	return c.JSON(http.StatusOK, nodes)
}

func (s *Server) htmxNodesTable(c echo.Context) error {
	nodes, err := s.db.ListNodes()
	if err != nil {
		return c.HTML(http.StatusInternalServerError, `<div class="alert alert-error">Could not load node list</div>`)
	}
	s.overlayNodeStatus(nodes)
	return renderPartial(c, "nodes_table", map[string]interface{}{"Nodes": nodes})
}

// htmxNodeMetrics returns real-time resource metrics for a specific node.
func (s *Server) htmxNodeMetrics(c echo.Context) error {
	nodeID := c.Param("id")
	node, err := s.db.GetNode(nodeID)
	if err != nil {
		return c.HTML(http.StatusNotFound, `<div class="text-error text-sm">Node not found</div>`)
	}
	if s.connector.IsAgentOnline(nodeID) {
		node.Status = "online"
	} else {
		node.Status = "offline"
	}

	data := s.getNodeMetricsData(nodeID, node)
	return renderPartial(c, "node_metrics", data)
}

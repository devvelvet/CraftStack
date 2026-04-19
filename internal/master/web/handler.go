package web

import (
	"fmt"
	"net/http"
	"regexp"

	"github.com/labstack/echo/v4"

	"craftstack/internal/master/store"
)

// instanceNameRe validates instance names: alphanumeric, hyphens, underscores, dots.
// Must start with alphanumeric. Docker container names use this as craftstack-{name}.
var instanceNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// --- state merge helper (in-memory stateful state basis) ---

// overlayNodeStatus overwrites node.Status based on live in-memory agent state.
// DB status may be stale; the in-memory agents map is the source of truth.
func (s *Server) overlayNodeStatus(nodes []*store.Node) {
	for _, n := range nodes {
		if s.connector.IsAgentOnline(n.ID) {
			n.Status = "online"
		} else {
			n.Status = "offline"
		}
	}
}

// overlayInstanceStatus adjusts instance status: if the owning node is offline,
// the instance cannot be running, so override to "offline".
func (s *Server) overlayInstanceStatus(instances []*store.Instance) {
	for _, inst := range instances {
		if !s.connector.IsAgentOnline(inst.NodeID) {
			// node offlineif instance offline
			inst.Status = "offline"
		}
	}
}

// --- page handler (HTML all page render) ---

func (s *Server) handleDashboard(c echo.Context) error {
	nodes, err := s.db.ListNodes()
	if err != nil {
		s.log.Error("node list query failed", "error", err)
	}
	s.overlayNodeStatus(nodes)

	instances, err := s.db.ListInstances("")
	if err != nil {
		s.log.Error("instance list query failed", "error", err)
	}
	s.overlayInstanceStatus(instances)

	syncHistory, err := s.db.ListSyncHistory(10)
	if err != nil {
		s.log.Error("sync history query failed", "error", err)
	}

	onlineNodes := 0
	for _, n := range nodes {
		if n.Status == "online" {
			onlineNodes++
		}
	}
	runningInstances := 0
	for _, i := range instances {
		if i.Status == "running" {
			runningInstances++
		}
	}

	// sync/backup aggregation
	allSync, _ := s.db.ListSyncHistory(0)
	totalBackups := 0
	for _, inst := range instances {
		count, _ := s.db.CountBackups(inst.ID)
		totalBackups += count
	}

	data := map[string]interface{}{
		"Title":            "dashboard",
		"TotalNodes":       len(nodes),
		"OnlineNodes":      onlineNodes,
		"TotalInstances":   len(instances),
		"RunningInstances": runningInstances,
		"TotalSyncs":       len(allSync),
		"TotalBackups":     totalBackups,
		"Nodes":            nodes,
		"Instances":        instances,
		"SyncHistory":      syncHistory,
	}
	return renderPage(c, "dashboard", data)
}

func (s *Server) handleNodes(c echo.Context) error {
	nodes, err := s.db.ListNodes()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	s.overlayNodeStatus(nodes)
	data := map[string]interface{}{
		"Title": "node manage",
		"Nodes": nodes,
	}
	return renderPage(c, "nodes", data)
}

func (s *Server) handleNodeDetail(c echo.Context) error {
	id := c.Param("id")
	node, err := s.db.GetNode(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "node not found")
	}
	// inmemory state drop
	if s.connector.IsAgentOnline(id) {
		node.Status = "online"
	} else {
		node.Status = "offline"
	}
	instances, err := s.db.ListInstances(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	s.overlayInstanceStatus(instances)
	syncHistory, _ := s.db.ListSyncHistoryByNode(id, 20)

	// metrics query (memory cache latest value)
	metricsData := s.getNodeMetricsData(id, node)

	data := map[string]interface{}{
		"Title":       fmt.Sprintf("node: %s", node.Name),
		"Node":        node,
		"Instances":   instances,
		"SyncHistory": syncHistory,
	}
	// metrics data merge
	for k, v := range metricsData {
		data[k] = v
	}

	return renderPage(c, "node_detail", data)
}

// getNodeMetricsData retrieves cached metrics for a node.
func (s *Server) getNodeMetricsData(nodeID string, node *store.Node) map[string]interface{} {
	data := map[string]interface{}{
		"CPUPercent":  float64(0),
		"MemPercent":  float64(0),
		"MemUsedMB":   int64(0),
		"MemTotalMB":  node.MemoryMB,
		"DiskPercent": float64(0),
		"DiskUsedMB":  int64(0),
		"DiskTotalMB": int64(0),
	}

	// connector from latest metrics query
	if mc, ok := s.connector.(MetricsProvider); ok {
		if m := mc.GetNodeMetrics(nodeID); m != nil {
			data["CPUPercent"] = m.CPUPercent
			data["MemPercent"] = m.MemPercent
			data["MemUsedMB"] = m.MemUsedMB
			data["MemTotalMB"] = m.MemTotalMB
			data["DiskPercent"] = m.DiskPercent
			data["DiskUsedMB"] = m.DiskUsedMB
			data["DiskTotalMB"] = m.DiskTotalMB
		}
	}

	return data
}

// parseIntSafe parses an int string safely.
func parseIntSafe(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

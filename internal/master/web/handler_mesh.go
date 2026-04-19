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
	"craftstack/internal/master/store"
)

// --- mesh network page handler ---

func (s *Server) handleMesh(c echo.Context) error {
	mesh, err := s.db.GetDefaultMesh()
	if err != nil {
		s.log.Warn("mesh network query failed", "error", err)
	}

	nodes, _ := s.db.ListNodes()
	s.overlayNodeStatus(nodes)

	dnsRecords, _ := s.db.ListDNSRecords("")

	data := map[string]interface{}{
		"Title":      "mesh network",
		"Mesh":       mesh,
		"Nodes":      nodes,
		"DNSRecords": dnsRecords,
	}
	return renderPage(c, "mesh", data)
}

// --- mesh API ---

// apiMeshStatus returns the mesh network status for all nodes.
func (s *Server) apiMeshStatus(c echo.Context) error {
	nodes, err := s.db.ListNodesWithWireGuard()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"status": "error", "message": err.Error()})
	}

	type nodeStatus struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		WGAddress    string `json:"wg_address"`
		WGEndpoint   string `json:"wg_endpoint"`
		DockerSubnet string `json:"docker_subnet"`
		Online       bool   `json:"online"`
		WGActive     bool   `json:"wg_active"`
	}

	var result []nodeStatus
	for _, n := range nodes {
		online := s.connector.IsAgentOnline(n.ID)
		result = append(result, nodeStatus{
			ID:           n.ID,
			Name:         n.Name,
			WGAddress:    n.WGAddress,
			WGEndpoint:   n.WGEndpoint,
			DockerSubnet: n.DockerSubnet,
			Online:       online,
			WGActive:     online, // WG active if online (assumed)
		})
	}

	return c.JSON(http.StatusOK, result)
}

// apiListDNSRecords returns all DNS records.
func (s *Server) apiListDNSRecords(c echo.Context) error {
	records, err := s.db.ListDNSRecords("")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"status": "error", "message": err.Error()})
	}
	return c.JSON(http.StatusOK, records)
}

// apiCreateDNSRecord manually creates a DNS record.
func (s *Server) apiCreateDNSRecord(c echo.Context) error {
	var req struct {
		Name       string `json:"name"`
		IPAddress  string `json:"ip_address"`
		InstanceID string `json:"instance_id"`
		NodeID     string `json:"node_id"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"status": "error", "message": "invalid request"})
	}

	if req.Name == "" || req.IPAddress == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"status": "error", "message": "name IP address required"})
	}

	mesh, err := s.db.GetDefaultMesh()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"status": "error", "message": "mesh network not found"})
	}

	fqdn := fmt.Sprintf("%s.%s", req.Name, mesh.Domain)
	if err := s.db.UpsertDNSRecord(&store.DNSRecord{
		MeshID:     mesh.ID,
		Name:       req.Name,
		FQDN:       fqdn,
		IPAddress:  req.IPAddress,
		InstanceID: req.InstanceID,
		NodeID:     req.NodeID,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"status": "error", "message": err.Error()})
	}

	// audit log: DNS record create
	s.audit(c, "create", "mesh", fqdn, req.Name, "", "", req.IPAddress,
		fmt.Sprintf("DNS record create: %s → %s", fqdn, req.IPAddress))

	return c.JSON(http.StatusOK, map[string]string{"status": "success", "message": "DNS record created"})
}

// apiDeleteDNSRecord deletes a DNS record by instance ID.
func (s *Server) apiDeleteDNSRecord(c echo.Context) error {
	instanceID := c.Param("instanceId")
	if instanceID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"status": "error", "message": "instance ID is required"})
	}

	if err := s.db.DeleteDNSRecordByInstance(instanceID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"status": "error", "message": err.Error()})
	}

	// audit log: DNS record delete
	s.audit(c, "delete", "mesh", instanceID, instanceID, "", "", "",
		fmt.Sprintf("DNS record delete: instance %s", instanceID))

	return c.JSON(http.StatusOK, map[string]string{"status": "success", "message": "DNS record deleted"})
}

// apiWireGuardStatus queries an agent for WireGuard status.
func (s *Server) apiWireGuardStatus(c echo.Context) error {
	nodeID := c.Param("id")

	agentAddr, ok := s.connector.GetAgentAddress(nodeID)
	if !ok {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "error", "message": "the agent offline"})
	}

	conn, err := grpc.NewClient(agentAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"status": "error", "message": err.Error()})
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(c.Request().Context(), 10*time.Second)
	defer cancel()

	client := pb.NewAgentServiceClient(conn)
	resp, err := client.WireGuardStatus(ctx, &pb.WireGuardStatusRequest{})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"status": "error", "message": err.Error()})
	}

	return c.JSON(http.StatusOK, resp)
}

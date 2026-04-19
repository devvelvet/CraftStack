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

// --- network page handler ---

func (s *Server) handleNetworks(c echo.Context) error {
	networks, err := s.db.ListNetworks("")
	if err != nil {
		s.log.Error("network list query failed", "error", err)
	}

	// each network connect instance count query
	var networksWithCount []networkWithCount
	for _, n := range networks {
		count, _ := s.db.CountNetworkInstances(n.ID)
		nodeName := n.NodeID
		if node, err := s.db.GetNode(n.NodeID); err == nil {
			nodeName = node.Name
		}
		networksWithCount = append(networksWithCount, networkWithCount{
			Network:       n,
			InstanceCount: count,
			NodeName:      nodeName,
		})
	}

	// online node list (create modal)
	nodes, _ := s.db.ListNodes()
	s.overlayNodeStatus(nodes)

	// all instance list (connect modal)
	instances, _ := s.db.ListInstances("")

	data := map[string]interface{}{
		"Title":     "network manage",
		"Networks":  networksWithCount,
		"Nodes":     nodes,
		"Instances": instances,
	}
	return renderPage(c, "networks", data)
}

// --- network API handler ---

// apiListNetworks returns all networks in DB.
func (s *Server) apiListNetworks(c echo.Context) error {
	nodeID := c.QueryParam("node_id")
	networks, err := s.db.ListNetworks(nodeID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	type networkInfo struct {
		*store.Network
		InstanceCount int `json:"instance_count"`
	}
	var result []networkInfo
	for _, n := range networks {
		count, _ := s.db.CountNetworkInstances(n.ID)
		result = append(result, networkInfo{Network: n, InstanceCount: count})
	}

	return c.JSON(http.StatusOK, result)
}

// apiCreateNetwork creates a Docker network on the specified agent and saves to DB.
func (s *Server) apiCreateNetwork(c echo.Context) error {
	var req struct {
		Name    string `json:"name"`
		Driver  string `json:"driver"`
		Subnet  string `json:"subnet"`
		Gateway string `json:"gateway"`
		NodeID  string `json:"node_id"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"status": "error", "message": "invalid request"})
	}

	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"status": "error", "message": "network name please enter"})
	}
	if req.NodeID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"status": "error", "message": "node please select"})
	}
	if req.Driver == "" {
		req.Driver = "bridge"
	}

	// agent create network request
	agentAddr, ok := s.connector.GetAgentAddress(req.NodeID)
	if !ok {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "error", "message": "the agent offline"})
	}

	conn, err := grpc.NewClient(agentAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"status": "error", "message": fmt.Sprintf("agent connection failed: %v", err)})
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(c.Request().Context(), 30*time.Second)
	defer cancel()

	agentClient := pb.NewAgentServiceClient(conn)
	resp, err := agentClient.CreateNetwork(ctx, &pb.CreateNetworkRequest{
		Name:    req.Name,
		Driver:  req.Driver,
		Subnet:  req.Subnet,
		Gateway: req.Gateway,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"status": "error", "message": fmt.Sprintf("create network failed: %v", err)})
	}
	if !resp.Success {
		return c.JSON(http.StatusInternalServerError, map[string]string{"status": "error", "message": resp.Message})
	}

	// DB network save
	networkID := fmt.Sprintf("%s-%s", req.NodeID, req.Name)
	if err := s.db.CreateNetwork(&store.Network{
		ID:      networkID,
		Name:    req.Name,
		Driver:  req.Driver,
		Subnet:  req.Subnet,
		Gateway: req.Gateway,
		NodeID:  req.NodeID,
	}); err != nil {
		s.log.Warn("network DB save failed (Docker already created)", "error", err)
	}

	// audit log: create network
	s.audit(c, "create", "network", networkID, req.Name, "", "", "",
		fmt.Sprintf("create network: %s (driver: %s, node: %s)", req.Name, req.Driver, req.NodeID))

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":     "success",
		"message":    "network created",
		"network_id": networkID,
	})
}

// apiDeleteNetwork deletes a network from the agent and DB.
func (s *Server) apiDeleteNetwork(c echo.Context) error {
	networkID := c.Param("id")

	network, err := s.db.GetNetwork(networkID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"status": "error", "message": "network not found"})
	}

	// connect the instance that has check
	count, _ := s.db.CountNetworkInstances(networkID)
	if count > 0 {
		return c.JSON(http.StatusConflict, map[string]string{
			"status":  "error",
			"message": fmt.Sprintf(" network %d the instance connectis set. first connect releaseplease.", count),
		})
	}

	// agent delete network request
	agentAddr, ok := s.connector.GetAgentAddress(network.NodeID)
	if ok {
		conn, err := grpc.NewClient(agentAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			defer conn.Close()
			ctx, cancel := context.WithTimeout(c.Request().Context(), 10*time.Second)
			defer cancel()
			agentClient := pb.NewAgentServiceClient(conn)
			resp, err := agentClient.DeleteNetwork(ctx, &pb.DeleteNetworkRequest{Name: network.Name})
			if err != nil {
				s.log.Warn("agent delete network failed", "error", err)
			} else if !resp.Success {
				s.log.Warn("agent delete network failed", "message", resp.Message)
			}
		}
	}

	// DB from delete
	if err := s.db.DeleteNetwork(networkID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"status": "error", "message": fmt.Sprintf("DB delete failed: %v", err)})
	}

	// audit log: delete network
	s.audit(c, "delete", "network", networkID, network.Name, "", "", "",
		fmt.Sprintf("delete network: %s", network.Name))

	return c.JSON(http.StatusOK, map[string]string{"status": "success", "message": "network deleted"})
}

// apiConnectNetwork connects an instance to a network.
func (s *Server) apiConnectNetwork(c echo.Context) error {
	networkID := c.Param("id")
	var req struct {
		InstanceID string `json:"instance_id"`
		Alias      string `json:"alias"`
		IPAddress  string `json:"ip_address"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"status": "error", "message": "invalid request"})
	}

	if req.InstanceID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"status": "error", "message": "instance please select"})
	}

	network, err := s.db.GetNetwork(networkID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"status": "error", "message": "network not found"})
	}

	inst, err := s.db.GetInstance(req.InstanceID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"status": "error", "message": "instance not found"})
	}

	// same node that has check
	if inst.NodeID != network.NodeID {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"status":  "error",
			"message": "instance network same node must exist ",
		})
	}

	containerName := fmt.Sprintf("craftstack-%s", inst.Name)

	// alias default: instance name
	alias := req.Alias
	if alias == "" {
		alias = inst.Name
	}

	// agent connect request
	agentAddr, ok := s.connector.GetAgentAddress(network.NodeID)
	if !ok {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "error", "message": "the agent offline"})
	}

	conn, err := grpc.NewClient(agentAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"status": "error", "message": fmt.Sprintf("agent connection failed: %v", err)})
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(c.Request().Context(), 10*time.Second)
	defer cancel()

	agentClient := pb.NewAgentServiceClient(conn)
	resp, err := agentClient.ConnectNetwork(ctx, &pb.ConnectNetworkRequest{
		NetworkName:   network.Name,
		ContainerName: containerName,
		Alias:         alias,
		IpAddress:     req.IPAddress,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"status": "error", "message": fmt.Sprintf("connect network failed: %v", err)})
	}
	if !resp.Success {
		return c.JSON(http.StatusInternalServerError, map[string]string{"status": "error", "message": resp.Message})
	}

	// DB mapping save
	if err := s.db.AddInstanceToNetwork(req.InstanceID, networkID, alias, req.IPAddress); err != nil {
		s.log.Warn("instance-network mapping DB save failed", "error", err)
	}

	// auto-register mesh DNS records (IPs may change when network connects)
	if s.mesh != nil {
		go s.mesh.RegisterInstanceDNS(inst)
	}

	// audit log: connect network
	s.audit(c, "connect", "network", networkID, network.Name, "", "", "",
		fmt.Sprintf("connect network: %s → %s", inst.Name, network.Name))

	return c.JSON(http.StatusOK, map[string]string{"status": "success", "message": "the instance network connected"})
}

// apiDisconnectNetwork disconnects an instance from a network.
func (s *Server) apiDisconnectNetwork(c echo.Context) error {
	networkID := c.Param("id")
	var req struct {
		InstanceID string `json:"instance_id"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"status": "error", "message": "invalid request"})
	}

	if req.InstanceID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"status": "error", "message": "instance please select"})
	}

	network, err := s.db.GetNetwork(networkID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"status": "error", "message": "network not found"})
	}

	inst, err := s.db.GetInstance(req.InstanceID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"status": "error", "message": "instance not found"})
	}

	containerName := fmt.Sprintf("craftstack-%s", inst.Name)

	// agent connect release request
	agentAddr, ok := s.connector.GetAgentAddress(network.NodeID)
	if ok {
		conn, err := grpc.NewClient(agentAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			defer conn.Close()
			ctx, cancel := context.WithTimeout(c.Request().Context(), 10*time.Second)
			defer cancel()
			agentClient := pb.NewAgentServiceClient(conn)
			resp, err := agentClient.DisconnectNetwork(ctx, &pb.DisconnectNetworkRequest{
				NetworkName:   network.Name,
				ContainerName: containerName,
			})
			if err != nil {
				s.log.Warn("agent connect network release failed", "error", err)
			} else if !resp.Success {
				s.log.Warn("agent connect network release failed", "message", resp.Message)
			}
		}
	}

	// DB from mapping delete
	if err := s.db.RemoveInstanceFromNetwork(req.InstanceID, networkID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"status": "error", "message": fmt.Sprintf("DB delete failed: %v", err)})
	}

	// refresh mesh DNS records (DNS removed for networks no longer present)
	if s.mesh != nil {
		remainingNets, _ := s.db.ListInstanceNetworks(req.InstanceID)
		if len(remainingNets) == 0 {
			s.mesh.UnregisterInstanceDNS(req.InstanceID)
		} else {
			go s.mesh.RegisterInstanceDNS(inst)
		}
	}

	// audit log: connect network release
	s.audit(c, "disconnect", "network", networkID, network.Name, "", "", "",
		fmt.Sprintf("connect network release: %s ← %s", inst.Name, network.Name))

	return c.JSON(http.StatusOK, map[string]string{"status": "success", "message": "the instance network from connect release"})
}

// --- HTMX min render ---

func (s *Server) htmxNetworksTable(c echo.Context) error {
	networks, err := s.db.ListNetworks("")
	if err != nil {
		return c.HTML(http.StatusInternalServerError, `<div class="alert alert-error">network list load cannot</div>`)
	}

	var networksData []networkWithCount
	for _, n := range networks {
		count, _ := s.db.CountNetworkInstances(n.ID)
		nodeName := n.NodeID
		if node, err := s.db.GetNode(n.NodeID); err == nil {
			nodeName = node.Name
		}
		networksData = append(networksData, networkWithCount{
			Network:       n,
			InstanceCount: count,
			NodeName:      nodeName,
		})
	}

	return renderPartial(c, "networks_table", map[string]interface{}{"Networks": networksData})
}

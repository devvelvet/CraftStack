package master

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"golang.org/x/crypto/curve25519"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "craftstack/gen/proto/craftstack"
	"craftstack/internal/master/store"
)

// MeshOrchestrator manages the WireGuard mesh network across all agents.
// It handles:
//   - Assigning WG addresses and Docker subnets to nodes
//   - Building peer lists for each agent
//   - Building DNS records for cross-node service discovery
//   - Pushing WG configuration and DNS records to agents
type MeshOrchestrator struct {
	db  *store.DB
	log *slog.Logger
	srv *GRPCServer
}

// NewMeshOrchestrator creates a new mesh orchestrator.
func NewMeshOrchestrator(db *store.DB, log *slog.Logger, srv *GRPCServer) *MeshOrchestrator {
	return &MeshOrchestrator{
		db:  db,
		log: log,
		srv: srv,
	}
}

// OnAgentRegistered is called when an agent registers or reconnects.
// It ensures the agent has WG keys and address assigned, then pushes config.
func (m *MeshOrchestrator) OnAgentRegistered(agentID string, agentGRPCAddr string) {
	node, err := m.db.GetNode(agentID)
	if err != nil {
		m.log.Warn("mesh: node query failed", "agent_id", agentID, "error", err)
		return
	}

	mesh, err := m.db.GetDefaultMesh()
	if err != nil || !mesh.Enabled {
		return
	}

	needsSetup := node.WGPublicKey == ""

	if needsSetup {
		m.log.Info("mesh: new node WireGuard settings allocate", "agent_id", agentID)

		// 1. WG install that has agent check (if absent auto install attempt)
		if err := m.ensureWireGuardInstalled(agentGRPCAddr); err != nil {
			m.log.Warn("mesh: WG install check failed", "agent_id", agentID, "error", err)
			return
		}

		// 2. master from X25519 keypair create
		privKey, pubKey, err := generateWireGuardKeyPair()
		if err != nil {
			m.log.Error("mesh: WG keypair create failed", "error", err)
			return
		}

		// 3. WG address and Docker subnet allocate
		idx, err := m.db.NextWGIndex()
		if err != nil {
			m.log.Error("mesh: WG index allocate failed", "error", err)
			return
		}

		wgAddress := fmt.Sprintf("10.10.0.%d/16", idx)
		dockerSubnet := fmt.Sprintf("172.30.%d.0/24", idx)

		// Derive endpoint from agent's gRPC address (use same host, WG port)
		wgEndpoint := m.deriveEndpoint(node.Address, 51820)

		// 4. DB keypair + address save
		if err := m.db.UpdateNodeWireGuard(agentID, pubKey, privKey, wgAddress, wgEndpoint, 51820, dockerSubnet); err != nil {
			m.log.Error("mesh: WG save settings failed", "error", err)
			return
		}

		node.WGPublicKey = pubKey
		node.WGPrivateKey = privKey
		node.WGAddress = wgAddress
		node.WGEndpoint = wgEndpoint
		node.WGListenPort = 51820
		node.DockerSubnet = dockerSubnet

		m.log.Info("mesh: WG settings allocate complete",
			"agent_id", agentID,
			"wg_address", wgAddress,
			"docker_subnet", dockerSubnet,
			"endpoint", wgEndpoint,
		)
	}

	// Push WG configuration to all agents (including the new one)
	go m.PushConfigToAllAgents()
}

// PushConfigToAllAgents sends updated WireGuard configuration to all online agents.
func (m *MeshOrchestrator) PushConfigToAllAgents() {
	nodes, err := m.db.ListNodesWithWireGuard()
	if err != nil {
		m.log.Error("mesh: WG node list query failed", "error", err)
		return
	}

	if len(nodes) == 0 {
		return
	}

	for _, node := range nodes {
		if !m.srv.IsAgentOnline(node.ID) {
			continue
		}

		peers := m.buildPeersFor(node.ID, nodes)
		m.pushConfigToAgent(node, peers)
	}
}

// BuildDNSRecords builds DNS records from all instances in the mesh network.
func (m *MeshOrchestrator) BuildDNSRecords() []*pb.DNSRecord {
	mesh, err := m.db.GetDefaultMesh()
	if err != nil || !mesh.Enabled {
		return nil
	}

	records, err := m.db.ListDNSRecords(mesh.ID)
	if err != nil {
		m.log.Warn("mesh: DNS record query failed", "error", err)
		return nil
	}

	var pbRecords []*pb.DNSRecord
	for _, r := range records {
		pbRecords = append(pbRecords, &pb.DNSRecord{
			Fqdn:       r.FQDN,
			IpAddress:  r.IPAddress,
			InstanceId: r.InstanceID,
			NodeId:     r.NodeID,
		})
	}
	return pbRecords
}

// RegisterInstanceDNS creates a DNS record for an instance.
// Called when an instance is created or its network info changes.
func (m *MeshOrchestrator) RegisterInstanceDNS(inst *store.Instance) {
	mesh, err := m.db.GetDefaultMesh()
	if err != nil || !mesh.Enabled {
		return
	}

	// Get the instance's Docker IP from instance_networks
	instNets, err := m.db.ListInstanceNetworks(inst.ID)
	if err != nil || len(instNets) == 0 {
		return
	}

	// Use alias or instance name as DNS name
	dnsName := inst.Name
	if instNets[0].Alias != "" {
		dnsName = instNets[0].Alias
	}

	// Use assigned IP or container name-based resolution
	ipAddr := instNets[0].IPAddress
	if ipAddr == "" {
		// No static IP assigned — we'll use the Docker container IP
		// which is obtained from docker inspect; for now, skip if not available
		return
	}

	fqdn := fmt.Sprintf("%s.%s", dnsName, mesh.Domain)

	if err := m.db.UpsertDNSRecord(&store.DNSRecord{
		MeshID:     mesh.ID,
		Name:       dnsName,
		FQDN:       fqdn,
		IPAddress:  ipAddr,
		InstanceID: inst.ID,
		NodeID:     inst.NodeID,
	}); err != nil {
		m.log.Warn("mesh: DNS record save failed", "error", err)
	}
}

// UnregisterInstanceDNS removes DNS records for an instance.
func (m *MeshOrchestrator) UnregisterInstanceDNS(instanceID string) {
	m.db.DeleteDNSRecordByInstance(instanceID)
}

// --- Internal Methods ---

// generateWireGuardKeyPair generates a WireGuard X25519 key pair in pure Go.
// Returns (privateKeyBase64, publicKeyBase64, error).
func generateWireGuardKeyPair() (string, string, error) {
	// Generate 32 bytes of random data for the private key
	var privBytes [32]byte
	if _, err := rand.Read(privBytes[:]); err != nil {
		return "", "", fmt.Errorf("random number generation failed: %w", err)
	}

	// Apply WireGuard key clamping (same as wg genkey)
	privBytes[0] &= 248
	privBytes[31] &= 127
	privBytes[31] |= 64

	// Derive public key via X25519 scalar multiplication
	pubBytes, err := curve25519.X25519(privBytes[:], curve25519.Basepoint)
	if err != nil {
		return "", "", fmt.Errorf("public key export failed: %w", err)
	}

	privKey := base64.StdEncoding.EncodeToString(privBytes[:])
	pubKey := base64.StdEncoding.EncodeToString(pubBytes)

	return privKey, pubKey, nil
}

// ensureWireGuardInstalled checks if WireGuard is installed on the agent,
// triggering auto-installation if needed.
func (m *MeshOrchestrator) ensureWireGuardInstalled(agentAddr string) error {
	conn, err := grpc.NewClient(agentAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("agent connection failed: %w", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := pb.NewAgentServiceClient(conn)

	// WireGuardStatus RPC will auto-install WG if not present
	resp, err := client.WireGuardStatus(ctx, &pb.WireGuardStatusRequest{})
	if err != nil {
		return fmt.Errorf("WG state check failed: %w", err)
	}

	if !resp.Installed {
		return fmt.Errorf("WG install failed or notinstall state")
	}

	return nil
}

// buildPeersFor builds the peer list for a specific node (excludes itself).
func (m *MeshOrchestrator) buildPeersFor(nodeID string, allNodes []*store.Node) []*pb.WireGuardPeer {
	var peers []*pb.WireGuardPeer
	for _, n := range allNodes {
		if n.ID == nodeID || n.WGPublicKey == "" {
			continue
		}

		// AllowedIPs: the peer's WG address + Docker subnet
		wgIP := strings.Split(n.WGAddress, "/")[0]
		allowedIPs := []string{wgIP + "/32"}
		if n.DockerSubnet != "" {
			allowedIPs = append(allowedIPs, n.DockerSubnet)
		}

		peers = append(peers, &pb.WireGuardPeer{
			PublicKey:  n.WGPublicKey,
			Endpoint:   n.WGEndpoint,
			AllowedIps: allowedIPs,
			Keepalive:  25,
		})
	}
	return peers
}

// pushConfigToAgent sends WireGuard configuration to a specific agent.
func (m *MeshOrchestrator) pushConfigToAgent(node *store.Node, peers []*pb.WireGuardPeer) {
	agentAddr, ok := m.srv.GetAgentAddress(node.ID)
	if !ok {
		return
	}

	conn, err := grpc.NewClient(agentAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		m.log.Warn("mesh: agent connection failed", "node", node.Name, "error", err)
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := pb.NewAgentServiceClient(conn)

	// Extract IP from WG address for DNS binding
	dnsIP := strings.Split(node.WGAddress, "/")[0]

	resp, err := client.ConfigureWireGuard(ctx, &pb.ConfigureWireGuardRequest{
		PrivateKey:  node.WGPrivateKey,
		Address:     node.WGAddress,
		ListenPort:  int32(node.WGListenPort),
		Peers:       peers,
		DnsListenIp: dnsIP,
	})
	if err != nil {
		m.log.Warn("mesh: WG settings send failed", "node", node.Name, "error", err)
		return
	}
	if !resp.Success {
		m.log.Warn("mesh: WG settings apply failed", "node", node.Name, "message", resp.Message)
		return
	}

	m.log.Info("mesh: WG settings send complete", "node", node.Name, "peers", len(peers))
}

// deriveEndpoint derives WireGuard endpoint from agent's gRPC address.
// Agent's gRPC address is like "192.168.1.10:50052", we extract the host
// and use WG port (51820).
func (m *MeshOrchestrator) deriveEndpoint(grpcAddr string, wgPort int) string {
	host := grpcAddr
	if idx := strings.LastIndex(grpcAddr, ":"); idx > 0 {
		host = grpcAddr[:idx]
	}
	// If host is 0.0.0.0 or empty, we can't determine public endpoint
	if host == "" || host == "0.0.0.0" || host == "::" {
		return ""
	}
	return fmt.Sprintf("%s:%d", host, wgPort)
}

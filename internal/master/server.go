package master

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	pb "craftstack/gen/proto/craftstack"
	"craftstack/internal/master/store"
	msync "craftstack/internal/master/sync"
)

// ConnectedAgent holds the state of a connected agent.
type ConnectedAgent struct {
	ID       string
	Name     string
	Address  string
	LastSeen time.Time
	Status   string
}

// GRPCServer manages gRPC connections from agents.
type GRPCServer struct {
	addr       string
	log        *slog.Logger
	db         *store.DB
	syncEngine *msync.Engine
	server     *grpc.Server

	mu     sync.RWMutex
	agents map[string]*ConnectedAgent

	// instance ID → agent ID inmemory mapping (heartbeat from refresh)
	instancesMu    sync.RWMutex
	instanceOwners map[string]string // instanceID -> agentID

	// node metrics cache (heartbeat from receive)
	metricsMu     sync.RWMutex
	cachedMetrics map[string]*CachedNodeMetrics

	// instance metrics cache (heartbeat from receive)
	instMetricsMu     sync.RWMutex
	cachedInstMetrics map[string]*CachedInstanceMetrics

	// Broadcast channels for real-time updates
	logBroadcast chan LogBroadcast

	// instanceper recent log ring buffer (WebSocket connect when audit log sent)
	logBufferMu sync.RWMutex
	logBuffers  map[string]*LogRingBuffer

	// gRPC service implementations
	metricsService *metricsServiceImpl

	// WireGuard mesh orchestrator
	meshOrch *MeshOrchestrator
}

// NewGRPCServer creates a new gRPC server for agent communication.
func NewGRPCServer(addr string, db *store.DB, syncEngine *msync.Engine, log *slog.Logger) *GRPCServer {
	srv := &GRPCServer{
		addr:              addr,
		log:               log,
		db:                db,
		syncEngine:        syncEngine,
		agents:            make(map[string]*ConnectedAgent),
		instanceOwners:    make(map[string]string),
		cachedMetrics:     make(map[string]*CachedNodeMetrics),
		cachedInstMetrics: make(map[string]*CachedInstanceMetrics),
		logBroadcast:      make(chan LogBroadcast, 1000),
		logBuffers:        make(map[string]*LogRingBuffer),
	}
	srv.meshOrch = NewMeshOrchestrator(db, log, srv)
	return srv
}

// MeshOrchestrator returns the mesh orchestrator for external use.
func (s *GRPCServer) MeshOrchestrator() *MeshOrchestrator {
	return s.meshOrch
}

// GetAgent returns a connected agent by ID.
func (s *GRPCServer) GetAgent(id string) (*ConnectedAgent, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.agents[id]
	return a, ok
}

// ListAgents returns all connected agents.
func (s *GRPCServer) ListAgents() []*ConnectedAgent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	agents := make([]*ConnectedAgent, 0, len(s.agents))
	for _, a := range s.agents {
		agents = append(agents, a)
	}
	return agents
}

// Start starts the gRPC server.
func (s *GRPCServer) Start() error {
	lis, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.addr, err)
	}

	s.server = grpc.NewServer(
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     5 * time.Minute,
			MaxConnectionAge:      30 * time.Minute,
			MaxConnectionAgeGrace: 10 * time.Second,
			Time:                  30 * time.Second,
			Timeout:               10 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             10 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.MaxRecvMsgSize(64*1024*1024), // 64MB for file transfers
	)

	// gRPC service implementationbody register
	pb.RegisterAgentServiceServer(s.server, &agentServiceImpl{srv: s, log: s.log})
	s.metricsService = &metricsServiceImpl{srv: s, log: s.log, agentConns: make(map[string]*grpc.ClientConn)}
	pb.RegisterMetricsServiceServer(s.server, s.metricsService)

	s.log.Info("gRPC server starting", "addr", s.addr)
	go func() {
		if err := s.server.Serve(lis); err != nil {
			s.log.Error("gRPC server error", "error", err)
		}
	}()

	// Start agent health check loop
	go s.healthCheckLoop()

	// Start instance metrics pruning loop (delete records older than 24h, every hour)
	go s.instanceMetricsPruneLoop()

	return nil
}

// Stop gracefully stops the gRPC server.
func (s *GRPCServer) Stop() {
	if s.server != nil {
		s.server.GracefulStop()
	}
}

// RegisterAgentConnection records a new agent connection.
func (s *GRPCServer) RegisterAgentConnection(id, name, address string) {
	s.mu.Lock()
	s.agents[id] = &ConnectedAgent{
		ID:       id,
		Name:     name,
		Address:  address,
		LastSeen: time.Now(),
		Status:   "online",
	}
	s.mu.Unlock()

	// Register in sync engine
	s.syncEngine.RegisterAgent(&msync.TargetAgent{
		ID:      id,
		Name:    name,
		Address: address,
	})

	// Update DB
	s.db.UpsertNode(&store.Node{
		ID:      id,
		Name:    name,
		Address: address,
		Status:  "online",
	})

	s.log.Info("agent connected", "id", id, "name", name, "address", address)
}

// UpdateAgentHeartbeat updates the last seen time for an agent.
func (s *GRPCServer) UpdateAgentHeartbeat(id string) {
	s.mu.Lock()
	if a, ok := s.agents[id]; ok {
		a.LastSeen = time.Now()
		a.Status = "online"
	}
	s.mu.Unlock()

	s.db.UpdateNodeStatus(id, "online")
}

// healthCheckLoop periodically checks agent health.
func (s *GRPCServer) healthCheckLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		for id, a := range s.agents {
			if time.Since(a.LastSeen) > 90*time.Second {
				a.Status = "offline"
				s.db.UpdateNodeStatus(id, "offline")
				s.syncEngine.UnregisterAgent(id)
				s.log.Warn("agent timed out", "id", id, "name", a.Name)

				// member instance offline as refresh
				if instances, err := s.db.ListInstances(id); err == nil {
					for _, inst := range instances {
						s.db.UpdateInstanceStatus(inst.ID, "offline", nil)
					}
				}
			}
		}
		s.mu.Unlock()
	}
}

// instanceMetricsPruneLoop periodically deletes old instance metrics records.
func (s *GRPCServer) instanceMetricsPruneLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		deleted, err := s.db.PruneInstanceMetrics(24 * time.Hour)
		if err != nil {
			s.log.Warn("instance metrics cleanup failed", "error", err)
		} else if deleted > 0 {
			s.log.Info("instance metrics cleanup complete", "deleted", deleted)
		}
	}
}

// IsAgentOnline returns true if the agent is connected and healthy (in-memory state).
func (s *GRPCServer) IsAgentOnline(agentID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.agents[agentID]
	if !ok {
		return false
	}
	return a.Status == "online"
}

// GetAgentAddress returns the gRPC address of a connected agent.
func (s *GRPCServer) GetAgentAddress(agentID string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.agents[agentID]
	if !ok {
		return "", false
	}
	return a.Address, true
}

// ListAgentIDs returns all connected agent IDs.
func (s *GRPCServer) ListAgentIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.agents))
	for id := range s.agents {
		ids = append(ids, id)
	}
	return ids
}

// SetAgentConn registers an agent's gRPC connection for command forwarding.
func (s *GRPCServer) SetAgentConn(agentID string, conn *grpc.ClientConn) {
	if s.metricsService != nil {
		s.metricsService.SetAgentConn(agentID, conn)
	}
}

// RemoveAgentConn removes an agent's gRPC connection.
func (s *GRPCServer) RemoveAgentConn(agentID string) {
	if s.metricsService != nil {
		s.metricsService.RemoveAgentConn(agentID)
	}
}

// Shutdown performs graceful shutdown.
func (s *GRPCServer) Shutdown(_ context.Context) {
	s.Stop()
}

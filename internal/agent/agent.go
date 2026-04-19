package agent

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/mem"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	pb "craftstack/gen/proto/craftstack"
	"craftstack/internal/agent/backup"
	agentdns "craftstack/internal/agent/dns"
	"craftstack/internal/agent/docker"
	"craftstack/internal/agent/metrics"
	"craftstack/internal/agent/process"
	"craftstack/internal/agent/wireguard"
	"craftstack/internal/common"
)

// Agent is the main agent that connects to the master and manages instances.
type Agent struct {
	cfg  *common.AgentConfig
	log  *slog.Logger
	conn *grpc.ClientConn

	// gRPC client (master service)
	agentClient   pb.AgentServiceClient
	metricsClient pb.MetricsServiceClient

	// Agent gRPC server (master from file receive and command received)
	grpcServer *grpc.Server

	backupMgr *backup.Manager
	collector *metrics.Collector
	dockerMgr *docker.Manager
	wgMgr     *wireguard.Manager
	dnsServer *agentdns.Server

	mu        sync.RWMutex
	instances map[string]process.Process
	defs      map[string]*common.InstanceDef // instID -> settings definition
	syncMu    sync.Mutex                     // syncInstances when execute prevent

	// WireGuard mesh status
	wgDNSListenIP string // WG IP for DNS binding

	ctx    context.Context
	cancel context.CancelFunc
}

// New creates a new Agent.
func New(cfg *common.AgentConfig, log *slog.Logger) *Agent {
	ctx, cancel := context.WithCancel(context.Background())

	return &Agent{
		cfg:       cfg,
		log:       log.With("agent_id", cfg.Agent.ID, "agent_name", cfg.Agent.Name),
		backupMgr: backup.NewManager(cfg.Backup.Dir, cfg.Backup.MaxCount, log),
		collector: metrics.NewCollector(log),
		dockerMgr: docker.NewManager(log),
		wgMgr:     wireguard.NewManager(log),
		instances: make(map[string]process.Process),
		defs:      make(map[string]*common.InstanceDef),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start connects to the master, starts the agent gRPC server, prepares instances, and begins operation.
func (a *Agent) Start() error {
	a.log.Info("start agent",
		"master_addr", a.cfg.Master.Addr,
		"grpc_addr", a.cfg.GRPC.Addr,
		"agent_id", a.cfg.Agent.ID,
		"instances", len(a.cfg.Instances),
	)

	// 0. Docker check + auto install/start
	if a.dockerMgr.IsInstalled() {
		if !a.dockerMgr.IsRunning() {
			a.log.Info("Docker install but daemon execute without . start attempt during...")
			if err := docker.EnsureDocker(a.ctx, a.log); err != nil {
				a.log.Warn("Docker daemon start failed — create instance when retry", "error", err)
			}
		} else {
			ver, _ := a.dockerMgr.Version(a.ctx)
			a.log.Info("Docker ready complete", "version", ver)
		}
	} else {
		a.log.Warn("Docker installis not set. create instance when auto install attempt")
	}

	// 1. Agent gRPC server start (master from file/command received)
	if err := a.startGRPCServer(); err != nil {
		return fmt.Errorf("Agent gRPC server start failed: %w", err)
	}

	// 2. master connect
	conn, err := grpc.NewClient(
		a.cfg.Master.Addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	)
	if err != nil {
		return fmt.Errorf("master connect failed: %w", err)
	}
	a.conn = conn
	a.agentClient = pb.NewAgentServiceClient(conn)
	a.metricsClient = pb.NewMetricsServiceClient(conn)
	a.log.Info("master connect complete", "addr", a.cfg.Master.Addr)

	// 3. master register agent → response as instance list receive
	if err := a.registerWithMaster(); err != nil {
		a.log.Warn("master register failed (heartbeat from instance sync e.g.)", "error", err)
	}

	// 4. background loop start
	go a.heartbeatLoop()
	go a.autoRestartLoop()
	go a.logStreamLoop()

	return nil
}

// startGRPCServer starts the agent's gRPC server for receiving files and commands.
func (a *Agent) startGRPCServer() error {
	lis, err := net.Listen("tcp", a.cfg.GRPC.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", a.cfg.GRPC.Addr, err)
	}

	a.grpcServer = grpc.NewServer(
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    30 * time.Second,
			Timeout: 10 * time.Second,
		}),
		grpc.MaxRecvMsgSize(256*1024*1024), // 256MB (file upload/download support)
		grpc.MaxSendMsgSize(256*1024*1024), // 256MB
	)

	// AgentService: instance control (start/stop/restart/kill)
	pb.RegisterAgentServiceServer(a.grpcServer, &agentControlServiceImpl{
		agent: a,
		log:   a.log,
	})

	// SyncService: file receive
	baseDir := "." // instance work_dir basis (TODO: settings changeable)
	pb.RegisterSyncServiceServer(a.grpcServer, &syncServiceImpl{
		baseDir: baseDir,
		log:     a.log,
	})

	// MetricsService: console command received
	pb.RegisterMetricsServiceServer(a.grpcServer, &agentMetricsServiceImpl{
		agent: a,
		log:   a.log,
	})

	// FileManagerService: file navigate/edit
	pb.RegisterFileManagerServiceServer(a.grpcServer, &fileManagerServiceImpl{
		agent: a,
		log:   a.log,
	})

	go func() {
		a.log.Info("Agent gRPC server start", "addr", a.cfg.GRPC.Addr)
		if err := a.grpcServer.Serve(lis); err != nil {
			a.log.Error("Agent gRPC server error", "error", err)
		}
	}()

	return nil
}

// registerWithMaster registers this agent with the master.
func (a *Agent) registerWithMaster() error {
	// system info collect
	cpuCores := int32(runtime.NumCPU())
	var memoryMB int64
	if v, err := mem.VirtualMemory(); err == nil {
		memoryMB = int64(v.Total / 1024 / 1024)
	}
	osInfo := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)

	resp, err := a.agentClient.RegisterAgent(a.ctx, &pb.RegisterAgentRequest{
		AgentId:  a.cfg.Agent.ID,
		Name:     a.cfg.Agent.Name,
		Address:  a.cfg.GRPC.Addr,
		CpuCores: cpuCores,
		MemoryMb: memoryMB,
		OsInfo:   osInfo,
	})
	if err != nil {
		return fmt.Errorf("RegisterAgent RPC failed: %w", err)
	}

	a.log.Info("master register complete", "agent_id", resp.AgentId, "message", resp.Message,
		"assigned_instances", len(resp.AssignedInstances))

	// received instance list from master DB (async — heartbeat loop blocking prevent)
	a.log.Info("instance sync goroutine start", "count", len(resp.AssignedInstances))
	go a.syncInstances(resp.AssignedInstances)

	return nil
}

// Stop gracefully shuts down the agent and all Java server instances.
func (a *Agent) Stop() error {
	a.log.Info("agent shutdown during")

	// 1. first all Java instance stop send command (parallel)
	a.mu.RLock()
	var wg sync.WaitGroup
	for id, proc := range a.instances {
		if proc.State() != process.StateRunning && proc.State() != process.StateStarting {
			continue
		}

		def := a.defs[id]
		stopCmd := "stop"
		if def != nil && def.StopCommand != "" {
			stopCmd = def.StopCommand
		}

		wg.Add(1)
		go func(instID, cmd string, p process.Process) {
			defer wg.Done()
			a.log.Info("instance shutdown start", "id", instID, "stop_command", cmd)

			// stop send command
			if _, err := p.SendCommand(cmd); err != nil {
				a.log.Warn("shutdown send command failed, force shutdown", "id", instID, "error", err)
				p.Kill()
				return
			}

			// max 30s wait
			deadline := time.After(30 * time.Second)
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-deadline:
					a.log.Warn("shutdown wait time s, force shutdown", "id", instID)
					p.Kill()
					return
				case <-ticker.C:
					state := p.State()
					if state == process.StateStopped || state == process.StateCrashed {
						a.log.Info("instance shutdown complete", "id", instID)
						return
					}
				}
			}
		}(id, stopCmd, proc)
	}
	a.mu.RUnlock()

	// all instance shutdown complete wait
	wg.Wait()
	a.log.Info("all instance shutdown complete")

	// if there are remaining process if present force shutdown (zombie prevent)
	a.mu.RLock()
	for id, proc := range a.instances {
		if proc.State() == process.StateRunning || proc.State() == process.StateStarting || proc.State() == process.StateStopping {
			a.log.Warn("leftover leftover process force shutdown", "id", id)
			proc.Kill()
		}
	}
	a.mu.RUnlock()

	// 2. context cancel (background goroutine cleanup)
	a.cancel()

	// 3. gRPC server/client shutdown
	if a.grpcServer != nil {
		a.grpcServer.GracefulStop()
	}

	if a.conn != nil {
		a.conn.Close()
	}

	a.log.Info("agent shutdown complete")
	return nil
}

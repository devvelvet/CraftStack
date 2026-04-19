package master

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"time"

	pb "craftstack/gen/proto/craftstack"
	"craftstack/internal/master/store"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// agentServiceImpl implements pb.AgentServiceServer on the Master side.
// Handles agent registration, heartbeat, instance control/listing.
type agentServiceImpl struct {
	pb.UnimplementedAgentServiceServer
	srv *GRPCServer
	log *slog.Logger
}

// RegisterAgent is called once when an agent starts up.
func (s *agentServiceImpl) RegisterAgent(ctx context.Context, req *pb.RegisterAgentRequest) (*pb.RegisterAgentResponse, error) {
	s.log.Info("register agent request",
		"agent_id", req.AgentId,
		"name", req.Name,
		"address", req.Address,
		"cpu_cores", req.CpuCores,
		"memory_mb", req.MemoryMb,
	)

	// cleanup previous stale node with same name (triggered on UUID change)
	if existingNodes, err := s.srv.db.ListNodes(); err == nil {
		for _, n := range existingNodes {
			if n.Name == req.Name && n.ID != req.AgentId {
				s.log.Info("previous session stale node cleanup", "old_id", n.ID, "new_id", req.AgentId, "name", req.Name)
				s.srv.db.DeleteNode(n.ID)
			}
		}
	}

	// agent connection register (agents map, sync engine, DB)
	s.srv.RegisterAgentConnection(req.AgentId, req.Name, req.Address)

	// DB CPU/memory info save
	s.srv.db.UpsertNode(&store.Node{
		ID:       req.AgentId,
		Name:     req.Name,
		Address:  req.Address,
		Status:   "online",
		CPUCores: int(req.CpuCores),
		MemoryMB: req.MemoryMb,
		OSInfo:   req.OsInfo,
	})

	// DB from  agent allocate instance list query response include
	assignedInstances := s.buildAssignedInstances(req.AgentId)

	// mesh network: register agent when WireGuard settings allocate and deploy
	if s.srv.meshOrch != nil {
		go s.srv.meshOrch.OnAgentRegistered(req.AgentId, req.Address)
	}

	return &pb.RegisterAgentResponse{
		Success:           true,
		AgentId:           req.AgentId,
		Message:           "register complete",
		AssignedInstances: assignedInstances,
	}, nil
}

// Heartbeat is a bidirectional stream for health checks and status updates.
func (s *agentServiceImpl) Heartbeat(stream grpc.BidiStreamingServer[pb.HeartbeatRequest, pb.HeartbeatResponse]) error {
	var agentID string
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			s.log.Info("heartbeat stream shutdown (EOF)", "agent_id", agentID)
			return nil
		}
		if err != nil {
			s.log.Warn("heartbeat receive error", "agent_id", agentID, "error", err)
			return err
		}

		agentID = req.AgentId
		s.srv.UpdateAgentHeartbeat(req.AgentId)

		// instance state update: inmemory mapping refresh + DB statusonly update
		// DB source of truth, so do not overwrite settings (name, port, memory, etc)
		for _, inst := range req.Instances {
			s.srv.UpdateInstanceOwner(inst.InstanceId, req.AgentId)
			statusStr := instanceStateToString(inst.State)
			if err := s.srv.db.UpdateInstanceStatus(inst.InstanceId, statusStr, nil); err != nil {
				s.log.Debug("instance state update failed (ignore)", "instance", inst.InstanceId, "error", err)
			}

			// instance metrics cache + DB save
			if inst.CpuPercent > 0 || inst.MemoryUsedMb > 0 {
				cached := &CachedInstanceMetrics{
					CPUPercent:      inst.CpuPercent,
					MemoryUsedMB:    inst.MemoryUsedMb,
					MemoryLimitMB:   inst.MemoryLimitMb,
					NetRxBytes:      inst.NetRxBytes,
					NetTxBytes:      inst.NetTxBytes,
					BlockReadBytes:  inst.BlockReadBytes,
					BlockWriteBytes: inst.BlockWriteBytes,
				}
				s.srv.UpdateInstanceMetrics(inst.InstanceId, cached)

				// DB metrics history save
				s.srv.db.InsertInstanceMetrics(&store.InstanceMetricRecord{
					InstanceID:      inst.InstanceId,
					CPUPercent:      inst.CpuPercent,
					MemoryUsedMB:    inst.MemoryUsedMb,
					MemoryLimitMB:   inst.MemoryLimitMb,
					NetRxBytes:      inst.NetRxBytes,
					NetTxBytes:      inst.NetTxBytes,
					BlockReadBytes:  inst.BlockReadBytes,
					BlockWriteBytes: inst.BlockWriteBytes,
				})
			}
		}

		// OS metrics cache update
		if req.Status != nil {
			memTotalMB := req.Status.MemoryTotalMb
			if memTotalMB == 0 && req.Status.MemoryUsagePercent > 0 {
				memTotalMB = int64(float64(req.Status.MemoryUsedMb) / req.Status.MemoryUsagePercent * 100)
			}
			diskTotalMB := req.Status.DiskTotalMb
			diskUsedMB := req.Status.DiskUsedMb
			var diskPercent float64
			if diskTotalMB > 0 {
				diskPercent = float64(diskUsedMB) / float64(diskTotalMB) * 100
			}
			s.srv.UpdateNodeMetrics(req.AgentId, &CachedNodeMetrics{
				CPUPercent:  req.Status.CpuUsagePercent,
				MemPercent:  req.Status.MemoryUsagePercent,
				MemUsedMB:   req.Status.MemoryUsedMb,
				MemTotalMB:  memTotalMB,
				DiskPercent: diskPercent,
				DiskUsedMB:  diskUsedMB,
				DiskTotalMB: diskTotalMB,
			})
		}

		s.log.Info("heartbeat receive",
			"agent_id", req.AgentId,
			"instances", len(req.Instances),
		)

		// DB from latest instance list query response include (dynamic apply)
		assignedInstances := s.buildAssignedInstances(req.AgentId)

		// cross node DNS record (mesh network)
		var dnsRecords []*pb.DNSRecord
		if s.srv.meshOrch != nil {
			dnsRecords = s.srv.meshOrch.BuildDNSRecords()
		}

		// response send
		if err := stream.Send(&pb.HeartbeatResponse{
			Timestamp:         timestamppb.New(time.Now()),
			AssignedInstances: assignedInstances,
			DnsRecords:        dnsRecords,
		}); err != nil {
			s.log.Warn("heartbeat response send failed", "agent_id", agentID, "error", err)
			return err
		}
	}
}

// ControlInstance forwards a control command to the appropriate agent.
func (s *agentServiceImpl) ControlInstance(ctx context.Context, req *pb.ControlInstanceRequest) (*pb.ControlInstanceResponse, error) {
	s.log.Info("instance control command",
		"agent_id", req.AgentId,
		"instance_id", req.InstanceId,
		"action", req.Action,
	)

	// the agent connect that has check
	agent, ok := s.srv.GetAgent(req.AgentId)
	if !ok {
		return &pb.ControlInstanceResponse{
			Success: false,
			Message: "the agent offline",
		}, nil
	}

	// agent gRPC server as reverse connect command forward
	conn, err := grpc.NewClient(agent.Address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return &pb.ControlInstanceResponse{
			Success: false,
			Message: "agent connection failed: " + err.Error(),
		}, nil
	}
	defer conn.Close()

	fwdCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	agentClient := pb.NewAgentServiceClient(conn)
	resp, err := agentClient.ControlInstance(fwdCtx, req)
	if err != nil {
		return &pb.ControlInstanceResponse{
			Success: false,
			Message: "command forward failed: " + err.Error(),
		}, nil
	}

	return resp, nil
}

// ListInstances returns all instances for a specific agent.
func (s *agentServiceImpl) ListInstances(ctx context.Context, req *pb.ListInstancesRequest) (*pb.ListInstancesResponse, error) {
	instances, err := s.srv.db.ListInstances(req.AgentId)
	if err != nil {
		return nil, err
	}

	var pbInstances []*pb.InstanceStatus
	for _, inst := range instances {
		instType := inst.InstanceType
		if instType == "" {
			instType = "minecraft"
		}
		pbInstances = append(pbInstances, &pb.InstanceStatus{
			InstanceId:   inst.ID,
			Name:         inst.Name,
			State:        stringToInstanceState(inst.Status),
			InstanceType: instType,
		})
	}

	return &pb.ListInstancesResponse{
		Instances: pbInstances,
	}, nil
}

func instanceStateToString(state pb.InstanceState) string {
	switch state {
	case pb.InstanceState_INSTANCE_STATE_STOPPED:
		return "stopped"
	case pb.InstanceState_INSTANCE_STATE_STARTING:
		return "starting"
	case pb.InstanceState_INSTANCE_STATE_RUNNING:
		return "running"
	case pb.InstanceState_INSTANCE_STATE_STOPPING:
		return "stopping"
	case pb.InstanceState_INSTANCE_STATE_CRASHED:
		return "crashed"
	default:
		return "unknown"
	}
}

// buildAssignedInstances retrieves instances for an agent from DB and converts to proto.
func (s *agentServiceImpl) buildAssignedInstances(agentID string) []*pb.InstanceConfig {
	instances, err := s.srv.db.ListInstances(agentID)
	if err != nil {
		s.log.Warn("instance list query failed", "agent_id", agentID, "error", err)
		return nil
	}

	var configs []*pb.InstanceConfig
	for _, inst := range instances {
		var jvmArgs []string
		if inst.JVMArgs != "" {
			for _, line := range strings.Split(inst.JVMArgs, "\n") {
				arg := strings.TrimSpace(line)
				if arg != "" {
					jvmArgs = append(jvmArgs, arg)
				}
			}
		}

		instType := inst.InstanceType
		if instType == "" {
			instType = "minecraft"
		}
		//  the instance connect network name query
		networkName := ""
		if instNetworks, err := s.srv.db.ListInstanceNetworks(inst.ID); err == nil && len(instNetworks) > 0 {
			// first th network name use (default network)
			if net, err := s.srv.db.GetNetwork(instNetworks[0].NetworkID); err == nil {
				networkName = net.Name
			}
		}

		configs = append(configs, &pb.InstanceConfig{
			InstanceId:       inst.ID,
			Name:             inst.Name,
			Port:             int32(inst.Port),
			MemoryMin:        inst.MemoryMin,
			MemoryMax:        inst.MemoryMax,
			ServerJar:        inst.ServerJar,
			WorkDir:          inst.WorkDir,
			JavaPath:         inst.JavaPath,
			AutoStart:        inst.AutoStart,
			AutoRestart:      inst.AutoRestart,
			RestartDelaySec:  int32(inst.RestartDelaySec),
			StopCommand:      inst.StopCommand,
			JvmArgs:          jvmArgs,
			AcceptEula:       inst.AcceptEULA,
			InstanceType:     instType,
			ServiceVersion:   inst.ServiceVersion,
			JavaVersion:      inst.JavaVersion,
			CustomDockerfile: inst.CustomDockerfile,
			CustomCompose:    inst.CustomCompose,
			NetworkName:      networkName,
			DockerMemory:     inst.DockerMemory,
			DockerCpus:       inst.DockerCPUs,
		})
	}
	return configs
}

func stringToInstanceState(s string) pb.InstanceState {
	switch s {
	case "stopped":
		return pb.InstanceState_INSTANCE_STATE_STOPPED
	case "starting":
		return pb.InstanceState_INSTANCE_STATE_STARTING
	case "running":
		return pb.InstanceState_INSTANCE_STATE_RUNNING
	case "stopping":
		return pb.InstanceState_INSTANCE_STATE_STOPPING
	case "crashed":
		return pb.InstanceState_INSTANCE_STATE_CRASHED
	default:
		return pb.InstanceState_INSTANCE_STATE_UNKNOWN
	}
}

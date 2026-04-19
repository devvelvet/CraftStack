package agent

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	pb "craftstack/gen/proto/craftstack"
	"craftstack/internal/agent/process"
)

// heartbeatLoop sends periodic heartbeats to the master via bidirectional streaming.
func (a *Agent) heartbeatLoop() {
	for {
		select {
		case <-a.ctx.Done():
			return
		default:
		}

		if err := a.runHeartbeatStream(); err != nil {
			a.log.Warn("heartbeat stream shutdown, reconnect wait", "error", err)
			select {
			case <-a.ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}
}

func (a *Agent) runHeartbeatStream() error {
	stream, err := a.agentClient.Heartbeat(a.ctx)
	if err != nil {
		return fmt.Errorf("heartbeat stream create failed: %w", err)
	}

	// receive error Send loop for passing channel
	recvErr := make(chan error, 1)

	// response receive goroutine
	go func() {
		for {
			resp, err := stream.Recv()
			if err != nil {
				recvErr <- err
				return
			}
			// master DB instance list sync (async — recv loop blocking prevent)
			if len(resp.AssignedInstances) > 0 {
				go a.syncInstances(resp.AssignedInstances)
			}
			// cross node DNS record sync
			if len(resp.DnsRecords) > 0 {
				a.updateDNSRecords(resp.DnsRecords)
			}
			// TODO: resp.Commands in wait command handling
		}
	}()

	// first heartbeat immediately send
	req := a.buildHeartbeatRequest()
	if err := stream.Send(req); err != nil {
		return fmt.Errorf("first heartbeat send failed: %w", err)
	}
	a.log.Info("first heartbeat send complete")

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			stream.CloseSend()
			return nil
		case err := <-recvErr:
			return fmt.Errorf("heartbeat receive error: %w", err)
		case <-ticker.C:
			req := a.buildHeartbeatRequest()
			if err := stream.Send(req); err != nil {
				return fmt.Errorf("heartbeat send failed: %w", err)
			}
			a.log.Info("heartbeat send complete", "instances", len(req.Instances))
		}
	}
}

func (a *Agent) buildHeartbeatRequest() *pb.HeartbeatRequest {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var instances []*pb.InstanceStatus
	for id, proc := range a.instances {
		instStatus := &pb.InstanceStatus{
			InstanceId:    id,
			Name:          proc.Name(),
			State:         processStateToProto(proc.State()),
			UptimeSeconds: int64(proc.Uptime().Seconds()),
		}
		// instance settings info include (master DB auto register)
		if def, ok := a.defs[id]; ok {
			instStatus.Port = int32(def.Port)
			instStatus.MemoryMin = def.MemoryMin
			instStatus.MemoryMax = def.MemoryMax
			instStatus.ServerJar = def.ServerJar
			instStatus.WorkDir = def.WorkDir
			instStatus.JavaPath = def.JavaPath
			instType := def.Type
			if instType == "" {
				instType = "minecraft"
			}
			instStatus.InstanceType = instType
		}
		// Docker container resource metrics collect (running stateonly)
		if proc.State() == process.StateRunning {
			containerName := fmt.Sprintf("craftstack-%s", proc.Name())
			if stats, err := a.dockerMgr.Stats(a.ctx, containerName); err == nil && stats != nil {
				instStatus.CpuPercent = stats.CPUPercent
				instStatus.MemoryUsedMb = stats.MemoryUsageMB
				instStatus.MemoryLimitMb = stats.MemoryLimitMB
				instStatus.NetRxBytes = stats.NetRxBytes
				instStatus.NetTxBytes = stats.NetTxBytes
				instStatus.BlockReadBytes = stats.BlockReadBytes
				instStatus.BlockWriteBytes = stats.BlockWriteBytes
			}
		}

		instances = append(instances, instStatus)
	}

	// OS metrics
	var agentStatus *pb.AgentStatus
	if osMetrics, err := a.collector.CollectOS(); err == nil {
		diskFreeMB := osMetrics.DiskTotalMB - osMetrics.DiskUsedMB
		var memPercent float64
		if osMetrics.MemoryTotalMB > 0 {
			memPercent = float64(osMetrics.MemoryUsedMB) / float64(osMetrics.MemoryTotalMB) * 100
		}
		var diskPercent float64
		if osMetrics.DiskTotalMB > 0 {
			diskPercent = float64(osMetrics.DiskUsedMB) / float64(osMetrics.DiskTotalMB) * 100
		}
		_ = diskPercent // the proto has percent field none, total/used as calculate
		agentStatus = &pb.AgentStatus{
			CpuUsagePercent:    osMetrics.CPUUsagePercent,
			MemoryUsagePercent: memPercent,
			MemoryUsedMb:       osMetrics.MemoryUsedMB,
			MemoryTotalMb:      osMetrics.MemoryTotalMB,
			DiskFreeMb:         diskFreeMB,
			DiskTotalMb:        osMetrics.DiskTotalMB,
			DiskUsedMb:         osMetrics.DiskUsedMB,
		}
	}

	return &pb.HeartbeatRequest{
		AgentId:   a.cfg.Agent.ID,
		Timestamp: timestamppb.Now(),
		Status:    agentStatus,
		Instances: instances,
	}
}

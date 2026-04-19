package master

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"

	pb "craftstack/gen/proto/craftstack"

	"google.golang.org/grpc"
)

// metricsServiceImpl implements pb.MetricsServiceServer on the Master side.
// Handles log streaming, metrics reporting, and console commands.
type metricsServiceImpl struct {
	pb.UnimplementedMetricsServiceServer
	srv *GRPCServer
	log *slog.Logger

	// Agent connections: agentID -> gRPC conn to agent
	// Used for forwarding commands to agents
	mu         sync.RWMutex
	agentConns map[string]*grpc.ClientConn
}

// SetAgentConn registers an agent's gRPC connection for command forwarding.
func (s *metricsServiceImpl) SetAgentConn(agentID string, conn *grpc.ClientConn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agentConns[agentID] = conn
}

// RemoveAgentConn removes an agent's gRPC connection.
func (s *metricsServiceImpl) RemoveAgentConn(agentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.agentConns, agentID)
}

// StreamLogs receives log streaming requests from the agent.
// The agent calls this to subscribe and then the master forwards log entries back.
// In practice, agents push logs via ReportMetrics or heartbeat;
// this serves web clients subscribing to an agent's logs.
func (s *metricsServiceImpl) StreamLogs(req *pb.StreamLogsRequest, stream grpc.ServerStreamingServer[pb.LogEntry]) error {
	s.log.Info("log stream subscribe request",
		"agent_id", req.AgentId,
		"instance_id", req.InstanceId,
		"tail_lines", req.TailLines,
	)

	// This will be implemented as a subscription to the log broadcast channel.
	// For now, keep the stream alive until client disconnects.
	<-stream.Context().Done()
	return stream.Context().Err()
}

// ReportMetrics receives periodic OS/JVM metrics from an agent as a client-streaming RPC.
func (s *metricsServiceImpl) ReportMetrics(stream grpc.ClientStreamingServer[pb.MetricsReport, pb.MetricsAck]) error {
	var count int32
	for {
		report, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&pb.MetricsAck{
				Success:       true,
				ReceivedCount: count,
			})
		}
		if err != nil {
			return err
		}

		count++

		s.log.Debug("metrics receive",
			"agent_id", report.AgentId,
			"instance_id", report.InstanceId,
		)

		// OS metrics broadcast (realtime update to web dashboard)
		if report.OsMetrics != nil {
			s.srv.BroadcastLog(report.InstanceId, fmt.Sprintf(
				"[metrics] CPU: %.1f%%, memory: %dMB/%dMB",
				report.OsMetrics.CpuUsagePercent,
				report.OsMetrics.MemoryUsedMb,
				report.OsMetrics.MemoryTotalMb,
			))
		}

		// log broadcast (realtime console streaming)
		for _, logLine := range report.LogLines {
			s.srv.BroadcastLog(report.InstanceId, logLine)
		}
	}
}

// SendCommand forwards a console command from the master to an agent's Minecraft stdin.
func (s *metricsServiceImpl) SendCommand(ctx context.Context, req *pb.ConsoleCommandRequest) (*pb.ConsoleCommandResponse, error) {
	s.log.Info("console command forward",
		"agent_id", req.AgentId,
		"instance_id", req.InstanceId,
		"command", req.Command,
	)

	// agent connection query
	s.mu.RLock()
	conn, ok := s.agentConns[req.AgentId]
	s.mu.RUnlock()

	if !ok {
		return &pb.ConsoleCommandResponse{
			Success: false,
			Error:   "the agent connectis not set",
		}, nil
	}

	// agent MetricsService.SendCommand as forward
	agentClient := pb.NewMetricsServiceClient(conn)
	resp, err := agentClient.SendCommand(ctx, req)
	if err != nil {
		return &pb.ConsoleCommandResponse{
			Success: false,
			Error:   fmt.Sprintf("agent command forward failed: %v", err),
		}, nil
	}

	return resp, nil
}

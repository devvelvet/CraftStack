package agent

import (
	"context"
	"fmt"
	"log/slog"

	pb "craftstack/gen/proto/craftstack"

	"google.golang.org/grpc"
)

// agentMetricsServiceImpl implements pb.MetricsServiceServer on the Agent side.
// Only SendCommand is meaningful on the agent; StreamLogs and ReportMetrics
// are client-to-server (agent pushes to master), not served by agent.
type agentMetricsServiceImpl struct {
	pb.UnimplementedMetricsServiceServer
	agent *Agent
	log   *slog.Logger
}

// SendCommand receives a console command from master and sends it to the instance.
// Returns the actual command output (for DB instances: query results, for Minecraft: empty).
func (s *agentMetricsServiceImpl) SendCommand(ctx context.Context, req *pb.ConsoleCommandRequest) (*pb.ConsoleCommandResponse, error) {
	s.log.Info("console command received",
		"instance_id", req.InstanceId,
		"command", req.Command,
	)

	output, err := s.agent.SendCommand(req.InstanceId, req.Command)
	if err != nil {
		return &pb.ConsoleCommandResponse{
			Success: false,
			Error:   fmt.Sprintf("command execution failed: %v", err),
		}, nil
	}

	if output == "" {
		output = "command sent"
	}

	return &pb.ConsoleCommandResponse{
		Success: true,
		Output:  output,
	}, nil
}

// StreamLogs is a no-op on agent side (logs flow agent -> master).
func (s *agentMetricsServiceImpl) StreamLogs(req *pb.StreamLogsRequest, stream grpc.ServerStreamingServer[pb.LogEntry]) error {
	<-stream.Context().Done()
	return stream.Context().Err()
}

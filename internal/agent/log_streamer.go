package agent

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	pb "craftstack/gen/proto/craftstack"
	"craftstack/internal/agent/process"
)

// logStreamLoop reads logs from all instances and streams them to the master.
// periodically check each instance state and if if running log forward start.
// process restartwhen new logCh detect automatically new forwarder start.
func (a *Agent) logStreamLoop() {
	type forwarderInfo struct {
		cancel context.CancelFunc
		logCh  <-chan process.LogLine //  forwarder reads from channel
	}

	active := make(map[string]*forwarderInfo)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			// all forwarder abort
			for _, info := range active {
				info.cancel()
			}
			return
		case <-ticker.C:
			a.mu.RLock()
			for id, proc := range a.instances {
				currentCh := proc.LogChannel()

				if info, exists := active[id]; exists {
					// already forwarder but channel has changed (restart)
					// existing forwarder shutdownand new start
					if info.logCh == currentCh {
						continue // same channel → as-is as keep
					}
					// channel changed = process restart
					a.log.Info("restart instance detect, log forwarder restart", "instance", id)
					info.cancel()
					delete(active, id)
				}

				// running state skip if not
				if proc.State() != process.StateRunning {
					continue
				}

				// new forwarder start
				ctx, cancel := context.WithCancel(a.ctx)
				info := &forwarderInfo{
					cancel: cancel,
					logCh:  currentCh,
				}
				active[id] = info
				go func(instID string, ch <-chan process.LogLine, fCtx context.Context) {
					a.forwardLogs(instID, ch, fCtx)
					// forwarder shutdown when active from remove (next ticker from redetect available)
					a.mu.Lock()
					delete(active, instID)
					a.mu.Unlock()
				}(id, currentCh, ctx)
			}
			a.mu.RUnlock()
		}
	}
}

// forwardLogs reads log lines from a channel and streams them to the master.
// master connect if disconnected auto reconnect attempt.
func (a *Agent) forwardLogs(instanceID string, logCh <-chan process.LogLine, ctx context.Context) {
	for {
		err := a.runLogStream(instanceID, logCh, ctx)
		if err == nil {
			// normal shutdown (channel closed or context cancel)
			a.log.Debug("log stream normal shutdown", "instance", instanceID)
			return
		}

		a.log.Warn("log stream shutdown, reconnect wait", "instance", instanceID, "error", err)

		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
			// reconnect attempt
		}
	}
}

func (a *Agent) runLogStream(instanceID string, logCh <-chan process.LogLine, ctx context.Context) error {
	stream, err := a.metricsClient.ReportMetrics(ctx)
	if err != nil {
		a.log.Warn("metrics stream create failed,  aslocal logonly use", "error", err)
		return err
	}

	// send logs in batches (100ms or 10-line batch)
	var batch []string
	flushTicker := time.NewTicker(100 * time.Millisecond)
	defer flushTicker.Stop()

	sendBatch := func() error {
		if len(batch) == 0 {
			return nil
		}
		sendErr := stream.Send(&pb.MetricsReport{
			AgentId:    a.cfg.Agent.ID,
			InstanceId: instanceID,
			Timestamp:  timestamppb.Now(),
			LogLines:   batch,
		})
		if sendErr != nil {
			a.log.Debug("log sent failed", "error", sendErr)
			return sendErr
		}
		batch = batch[:0]
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			sendBatch()
			stream.CloseAndRecv()
			return nil

		case line, ok := <-logCh:
			if !ok {
				// channel closed = process shutdown
				sendBatch()
				stream.CloseAndRecv()
				return nil
			}
			a.log.Debug("server log", "instance", instanceID, "line", line.Line)
			batch = append(batch, line.Line)
			if len(batch) >= 10 {
				if err := sendBatch(); err != nil {
					return fmt.Errorf("log batch send failed: %w", err)
				}
			}

		case <-flushTicker.C:
			if err := sendBatch(); err != nil {
				return fmt.Errorf("log flush send failed: %w", err)
			}
		}
	}
}

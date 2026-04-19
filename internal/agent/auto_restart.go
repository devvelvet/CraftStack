package agent

import (
	"time"

	"craftstack/internal/agent/process"
)

// autoRestartLoop monitors instances and restarts crashed ones if auto_restart is enabled.
// Implements exponential backoff to prevent crash loops.
func (a *Agent) autoRestartLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// prevent crash loop: per-instance consecutive failure count and last attempt time
	crashCounts := make(map[string]int)
	lastAttempt := make(map[string]time.Time)
	const maxCrashCount = 5 //  count s when auto-restart aborted

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.mu.RLock()
			for id, proc := range a.instances {
				def := a.defs[id]
				if def == nil || !def.AutoRestart {
					continue
				}
				if proc.State() != process.StateCrashed {
					// normal runningif count reset
					if proc.State() == process.StateRunning {
						crashCounts[id] = 0
					}
					continue
				}

				count := crashCounts[id]
				if count >= maxCrashCount {
					// 5x consecutive failures when auto-restart aborted
					if count == maxCrashCount {
						a.log.Error("auto-restart aborted: consecutive failures count s and",
							"instance", def.Name,
							"crash_count", count,
							"hint", "please verify JAR file and Java path")
						crashCounts[id] = count + 1 // log duplicate prevent
					}
					continue
				}

				// exponential backoff: delay * 2^count (max 5min)
				baseDelay := time.Duration(def.RestartDelaySec) * time.Second
				backoff := baseDelay * (1 << uint(count))
				if backoff > 5*time.Minute {
					backoff = 5 * time.Minute
				}

				// check if enough time has passed since last attempt
				if last, ok := lastAttempt[id]; ok && time.Since(last) < backoff {
					continue
				}

				crashCounts[id] = count + 1
				lastAttempt[id] = time.Now()

				a.log.Warn("abnormal shutdown detect, auto-restart",
					"instance", def.Name,
					"crash_count", count+1,
					"max_retries", maxCrashCount,
					"backoff", backoff.String(),
				)

				go func(instID string, delay time.Duration) {
					select {
					case <-a.ctx.Done():
						return
					case <-time.After(delay):
					}
					if err := a.ControlInstance(instID, "start"); err != nil {
						a.log.Error("auto-restart failed", "instance", instID, "error", err)
					}
				}(id, backoff)
			}
			a.mu.RUnlock()
		}
	}
}

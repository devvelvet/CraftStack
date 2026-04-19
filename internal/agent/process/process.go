package process

import "time"

// Process is the common interface for all managed service processes.
// Implemented by JavaProcess (Minecraft, Kafka) and GenericProcess (MySQL, PostgreSQL, MongoDB, Redis).
type Process interface {
	// ID returns the unique instance identifier.
	ID() string
	// Name returns the human-readable instance name.
	Name() string
	// InstanceType returns the service type (minecraft, mysql, postgresql, mongodb, redis, kafka).
	InstanceType() string
	// State returns the current lifecycle state.
	State() State
	// PID returns the OS process ID (0 if not running).
	PID() int
	// Uptime returns how long the process has been running.
	Uptime() time.Duration
	// LogChannel returns a read-only channel of log output lines.
	LogChannel() <-chan LogLine

	// Start launches the service process.
	Start() error
	// Stop gracefully shuts down the service.
	Stop() error
	// Kill forcefully terminates the service.
	Kill() error
	// Restart performs a Stop followed by Start.
	Restart() error
	// SendCommand sends a command to the process stdin (if supported).
	// Returns the command output (if available) and any error.
	SendCommand(command string) (string, error)
}

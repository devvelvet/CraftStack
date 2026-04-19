package master

import "time"

// LogRingBuffer is a fixed-size circular buffer for log lines.
type LogRingBuffer struct {
	lines []string
	pos   int
	full  bool
	size  int
}

// NewLogRingBuffer creates a ring buffer with the given capacity.
func NewLogRingBuffer(size int) *LogRingBuffer {
	return &LogRingBuffer{
		lines: make([]string, size),
		size:  size,
	}
}

// Add appends a line to the ring buffer.
func (rb *LogRingBuffer) Add(line string) {
	rb.lines[rb.pos] = line
	rb.pos++
	if rb.pos >= rb.size {
		rb.pos = 0
		rb.full = true
	}
}

// Lines returns all buffered lines in chronological order.
func (rb *LogRingBuffer) Lines() []string {
	if !rb.full {
		return rb.lines[:rb.pos]
	}
	result := make([]string, 0, rb.size)
	result = append(result, rb.lines[rb.pos:]...)
	result = append(result, rb.lines[:rb.pos]...)
	return result
}

// LogBroadcast carries a log line from agent to web clients.
type LogBroadcast struct {
	InstanceID string
	Line       string
	Timestamp  time.Time
}

// BroadcastLog sends a log line to the broadcast channel and stores it in the ring buffer.
func (s *GRPCServer) BroadcastLog(instanceID, line string) {
	// ring buffer storage (for audit log queries)
	s.logBufferMu.Lock()
	buf, ok := s.logBuffers[instanceID]
	if !ok {
		buf = NewLogRingBuffer(500)
		s.logBuffers[instanceID] = buf
	}
	buf.Add(line)
	s.logBufferMu.Unlock()

	select {
	case s.logBroadcast <- LogBroadcast{
		InstanceID: instanceID,
		Line:       line,
		Timestamp:  time.Now(),
	}:
	default:
		// Channel full, drop
	}
}

// GetLogHistory returns the buffered log lines for an instance.
func (s *GRPCServer) GetLogHistory(instanceID string) []string {
	s.logBufferMu.RLock()
	defer s.logBufferMu.RUnlock()
	buf, ok := s.logBuffers[instanceID]
	if !ok {
		return nil
	}
	return buf.Lines()
}

// LogBroadcasts returns the channel for receiving log broadcasts.
func (s *GRPCServer) LogBroadcasts() <-chan LogBroadcast {
	return s.logBroadcast
}

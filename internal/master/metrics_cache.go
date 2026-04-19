package master

import (
	"time"

	"craftstack/internal/master/web"
)

// CachedInstanceMetrics holds the latest metrics received for an instance via heartbeat.
type CachedInstanceMetrics struct {
	CPUPercent      float64   `json:"cpu_percent"`
	MemoryUsedMB    int64     `json:"memory_used_mb"`
	MemoryLimitMB   int64     `json:"memory_limit_mb"`
	NetRxBytes      int64     `json:"net_rx_bytes"`
	NetTxBytes      int64     `json:"net_tx_bytes"`
	BlockReadBytes  int64     `json:"block_read_bytes"`
	BlockWriteBytes int64     `json:"block_write_bytes"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// CachedNodeMetrics holds the latest metrics received from an agent via heartbeat.
type CachedNodeMetrics struct {
	CPUPercent  float64
	MemPercent  float64
	MemUsedMB   int64
	MemTotalMB  int64
	DiskPercent float64
	DiskUsedMB  int64
	DiskTotalMB int64
	UpdatedAt   time.Time
}

// UpdateNodeMetrics updates the cached metrics for a node (called from heartbeat).
func (s *GRPCServer) UpdateNodeMetrics(agentID string, m *CachedNodeMetrics) {
	s.metricsMu.Lock()
	defer s.metricsMu.Unlock()
	m.UpdatedAt = time.Now()
	s.cachedMetrics[agentID] = m
}

// GetNodeMetrics returns the cached metrics for a node.
// Implements web.MetricsProvider.
func (s *GRPCServer) GetNodeMetrics(nodeID string) *web.NodeMetrics {
	s.metricsMu.RLock()
	defer s.metricsMu.RUnlock()
	m, ok := s.cachedMetrics[nodeID]
	if !ok {
		return nil
	}
	return &web.NodeMetrics{
		CPUPercent:  m.CPUPercent,
		MemPercent:  m.MemPercent,
		MemUsedMB:   m.MemUsedMB,
		MemTotalMB:  m.MemTotalMB,
		DiskPercent: m.DiskPercent,
		DiskUsedMB:  m.DiskUsedMB,
		DiskTotalMB: m.DiskTotalMB,
	}
}

// UpdateInstanceMetrics updates the cached metrics for an instance (called from heartbeat).
func (s *GRPCServer) UpdateInstanceMetrics(instanceID string, m *CachedInstanceMetrics) {
	s.instMetricsMu.Lock()
	defer s.instMetricsMu.Unlock()
	m.UpdatedAt = time.Now()
	s.cachedInstMetrics[instanceID] = m
}

// GetInstanceMetrics returns the cached metrics for an instance.
// Implements web.InstanceMetricsProvider.
func (s *GRPCServer) GetInstanceMetrics(instanceID string) *web.InstanceMetrics {
	s.instMetricsMu.RLock()
	defer s.instMetricsMu.RUnlock()
	m, ok := s.cachedInstMetrics[instanceID]
	if !ok {
		return nil
	}
	return &web.InstanceMetrics{
		CPUPercent:      m.CPUPercent,
		MemoryUsedMB:    m.MemoryUsedMB,
		MemoryLimitMB:   m.MemoryLimitMB,
		NetRxBytes:      m.NetRxBytes,
		NetTxBytes:      m.NetTxBytes,
		BlockReadBytes:  m.BlockReadBytes,
		BlockWriteBytes: m.BlockWriteBytes,
		UpdatedAt:       m.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// UpdateInstanceOwner records which agent owns an instance (called from heartbeat).
func (s *GRPCServer) UpdateInstanceOwner(instanceID, agentID string) {
	s.instancesMu.Lock()
	defer s.instancesMu.Unlock()
	s.instanceOwners[instanceID] = agentID
}

// GetInstanceOwner returns the agent ID that owns an instance.
// Implements web.AgentConnector.FindAgentForInstance.
func (s *GRPCServer) GetInstanceOwner(instanceID string) (agentID string, ok bool) {
	s.instancesMu.RLock()
	defer s.instancesMu.RUnlock()
	agentID, ok = s.instanceOwners[instanceID]
	return
}

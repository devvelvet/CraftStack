package master

import (
	"craftstack/internal/master/observability"
)

// NodeSnapshots exposes cached node metrics to the observability exporter.
func (s *GRPCServer) NodeSnapshots() []observability.NodeSnapshot {
	s.metricsMu.RLock()
	defer s.metricsMu.RUnlock()
	out := make([]observability.NodeSnapshot, 0, len(s.cachedMetrics))
	for id, m := range s.cachedMetrics {
		out = append(out, observability.NodeSnapshot{
			NodeID:      id,
			CPUPercent:  m.CPUPercent,
			MemPercent:  m.MemPercent,
			MemUsedMB:   m.MemUsedMB,
			MemTotalMB:  m.MemTotalMB,
			DiskPercent: m.DiskPercent,
			DiskUsedMB:  m.DiskUsedMB,
			DiskTotalMB: m.DiskTotalMB,
			UpdatedAt:   m.UpdatedAt,
		})
	}
	return out
}

// InstanceSnapshots exposes cached instance metrics to the observability exporter.
func (s *GRPCServer) InstanceSnapshots() []observability.InstanceSnapshot {
	s.instMetricsMu.RLock()
	defer s.instMetricsMu.RUnlock()
	s.instancesMu.RLock()
	owners := make(map[string]string, len(s.instanceOwners))
	for k, v := range s.instanceOwners {
		owners[k] = v
	}
	s.instancesMu.RUnlock()

	out := make([]observability.InstanceSnapshot, 0, len(s.cachedInstMetrics))
	for id, m := range s.cachedInstMetrics {
		out = append(out, observability.InstanceSnapshot{
			InstanceID:      id,
			NodeID:          owners[id],
			CPUPercent:      m.CPUPercent,
			MemoryUsedMB:    m.MemoryUsedMB,
			MemoryLimitMB:   m.MemoryLimitMB,
			NetRxBytes:      m.NetRxBytes,
			NetTxBytes:      m.NetTxBytes,
			BlockReadBytes:  m.BlockReadBytes,
			BlockWriteBytes: m.BlockWriteBytes,
			UpdatedAt:       m.UpdatedAt,
		})
	}
	return out
}

package store

import (
	"fmt"
	"time"
)

// InstanceMetricRecord represents a single metrics snapshot for an instance.
type InstanceMetricRecord struct {
	ID              int       `json:"id"`
	InstanceID      string    `json:"instance_id"`
	CPUPercent      float64   `json:"cpu_percent"`
	MemoryUsedMB    int64     `json:"memory_used_mb"`
	MemoryLimitMB   int64     `json:"memory_limit_mb"`
	NetRxBytes      int64     `json:"net_rx_bytes"`
	NetTxBytes      int64     `json:"net_tx_bytes"`
	BlockReadBytes  int64     `json:"block_read_bytes"`
	BlockWriteBytes int64     `json:"block_write_bytes"`
	RecordedAt      time.Time `json:"recorded_at"`
}

// InsertInstanceMetrics saves a metrics snapshot.
func (d *DB) InsertInstanceMetrics(m *InstanceMetricRecord) error {
	_, err := d.Exec(`
		INSERT INTO instance_metrics (instance_id, cpu_percent, memory_used_mb, memory_limit_mb, net_rx_bytes, net_tx_bytes, block_read_bytes, block_write_bytes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		m.InstanceID, m.CPUPercent, m.MemoryUsedMB, m.MemoryLimitMB,
		m.NetRxBytes, m.NetTxBytes, m.BlockReadBytes, m.BlockWriteBytes,
	)
	return err
}

// ListInstanceMetrics returns recent metrics for an instance (for graphing).
// Results are ordered from oldest to newest.
func (d *DB) ListInstanceMetrics(instanceID string, limit int) ([]*InstanceMetricRecord, error) {
	if limit <= 0 {
		limit = 60
	}

	rows, err := d.Query(`
		SELECT id, instance_id, cpu_percent, memory_used_mb, memory_limit_mb,
		       net_rx_bytes, net_tx_bytes, block_read_bytes, block_write_bytes, recorded_at
		FROM instance_metrics
		WHERE instance_id = ?
		ORDER BY recorded_at DESC
		LIMIT ?`, instanceID, limit)
	if err != nil {
		return nil, fmt.Errorf("query instance_metrics: %w", err)
	}
	defer rows.Close()

	var records []*InstanceMetricRecord
	for rows.Next() {
		r := &InstanceMetricRecord{}
		if err := rows.Scan(&r.ID, &r.InstanceID, &r.CPUPercent, &r.MemoryUsedMB, &r.MemoryLimitMB,
			&r.NetRxBytes, &r.NetTxBytes, &r.BlockReadBytes, &r.BlockWriteBytes, &r.RecordedAt); err != nil {
			return nil, fmt.Errorf("scan instance_metrics: %w", err)
		}
		records = append(records, r)
	}

	// Reverse to chronological order (oldest first)
	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}

	return records, nil
}

// PruneInstanceMetrics deletes metrics older than the given duration.
// Returns the number of deleted rows.
func (d *DB) PruneInstanceMetrics(maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge)
	result, err := d.Exec(`DELETE FROM instance_metrics WHERE recorded_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("prune instance_metrics: %w", err)
	}
	return result.RowsAffected()
}

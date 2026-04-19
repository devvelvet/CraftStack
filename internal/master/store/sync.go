package store

import (
	"fmt"
	"time"
)

// SyncRecord represents a file synchronization history entry.
type SyncRecord struct {
	ID         int       `json:"id"`
	InstanceID *string   `json:"instance_id,omitempty"`
	NodeID     *string   `json:"node_id,omitempty"`
	FilePath   string    `json:"file_path"`
	FileSize   int64     `json:"file_size"`
	FileHash   string    `json:"file_hash"`
	Action     string    `json:"action"` // push, delete
	Status     string    `json:"status"` // pending, syncing, completed, failed
	ErrorMsg   *string   `json:"error_msg,omitempty"`
	SyncedAt   time.Time `json:"synced_at"`
}

// CreateSyncRecord inserts a new sync history entry.
func (d *DB) CreateSyncRecord(r *SyncRecord) error {
	result, err := d.Exec(`
		INSERT INTO sync_history (instance_id, node_id, file_path, file_size, file_hash, action, status, error_msg)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, r.InstanceID, r.NodeID, r.FilePath, r.FileSize, r.FileHash, r.Action, r.Status, r.ErrorMsg)
	if err != nil {
		return fmt.Errorf("create sync record: %w", err)
	}
	id, _ := result.LastInsertId()
	r.ID = int(id)
	return nil
}

// UpdateSyncStatus updates the status of a sync record.
func (d *DB) UpdateSyncStatus(id int, status string, errMsg *string) error {
	_, err := d.Exec(`
		UPDATE sync_history SET status = ?, error_msg = ?, synced_at = CURRENT_TIMESTAMP WHERE id = ?
	`, status, errMsg, id)
	if err != nil {
		return fmt.Errorf("update sync status: %w", err)
	}
	return nil
}

// ListSyncHistory returns recent sync history entries.
func (d *DB) ListSyncHistory(limit int) ([]*SyncRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := d.Query(`
		SELECT id, instance_id, node_id, file_path, COALESCE(file_size, 0),
		       file_hash, action, status, error_msg, synced_at
		FROM sync_history
		ORDER BY synced_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list sync history: %w", err)
	}
	defer rows.Close()

	var records []*SyncRecord
	for rows.Next() {
		r := &SyncRecord{}
		if err := rows.Scan(&r.ID, &r.InstanceID, &r.NodeID, &r.FilePath,
			&r.FileSize, &r.FileHash, &r.Action, &r.Status, &r.ErrorMsg, &r.SyncedAt); err != nil {
			return nil, fmt.Errorf("scan sync record: %w", err)
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// ListSyncHistoryByNode returns sync history filtered by node.
func (d *DB) ListSyncHistoryByNode(nodeID string, limit int) ([]*SyncRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.Query(`
		SELECT id, instance_id, node_id, file_path, COALESCE(file_size, 0),
		       file_hash, action, status, error_msg, synced_at
		FROM sync_history
		WHERE node_id = ?
		ORDER BY synced_at DESC
		LIMIT ?
	`, nodeID, limit)
	if err != nil {
		return nil, fmt.Errorf("list sync history by node: %w", err)
	}
	defer rows.Close()

	var records []*SyncRecord
	for rows.Next() {
		r := &SyncRecord{}
		if err := rows.Scan(&r.ID, &r.InstanceID, &r.NodeID, &r.FilePath,
			&r.FileSize, &r.FileHash, &r.Action, &r.Status, &r.ErrorMsg, &r.SyncedAt); err != nil {
			return nil, fmt.Errorf("scan sync record: %w", err)
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

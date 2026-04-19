package store

import (
	"fmt"
	"time"
)

// Backup represents a backup record.
type Backup struct {
	ID          int       `json:"id"`
	InstanceID  string    `json:"instance_id"`
	FilePath    string    `json:"file_path"`
	FileSize    int64     `json:"file_size"`
	Checksum    string    `json:"checksum"`
	TriggerType string    `json:"trigger_type"` // manual, scheduled, pre_sync
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

// CreateBackup inserts a new backup record.
func (d *DB) CreateBackup(b *Backup) error {
	result, err := d.Exec(`
		INSERT INTO backups (instance_id, file_path, file_size, checksum, trigger_type, status)
		VALUES (?, ?, ?, ?, ?, ?)
	`, b.InstanceID, b.FilePath, b.FileSize, b.Checksum, b.TriggerType, b.Status)
	if err != nil {
		return fmt.Errorf("create backup: %w", err)
	}
	id, _ := result.LastInsertId()
	b.ID = int(id)
	return nil
}

// ListBackups returns backups for an instance, ordered newest first.
func (d *DB) ListBackups(instanceID string, limit int) ([]*Backup, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.Query(`
		SELECT id, instance_id, file_path, file_size, COALESCE(checksum, ''),
		       trigger_type, status, created_at
		FROM backups
		WHERE instance_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, instanceID, limit)
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	defer rows.Close()

	var backups []*Backup
	for rows.Next() {
		b := &Backup{}
		if err := rows.Scan(&b.ID, &b.InstanceID, &b.FilePath, &b.FileSize,
			&b.Checksum, &b.TriggerType, &b.Status, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan backup: %w", err)
		}
		backups = append(backups, b)
	}
	return backups, rows.Err()
}

// CountBackups returns the number of backups for an instance.
func (d *DB) CountBackups(instanceID string) (int, error) {
	var count int
	err := d.QueryRow("SELECT COUNT(*) FROM backups WHERE instance_id = ?", instanceID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count backups: %w", err)
	}
	return count, nil
}

// DeleteOldestBackups deletes the oldest backups exceeding maxCount for an instance.
func (d *DB) DeleteOldestBackups(instanceID string, maxCount int) ([]string, error) {
	// Get file paths of backups to delete
	rows, err := d.Query(`
		SELECT file_path FROM backups
		WHERE instance_id = ?
		ORDER BY created_at DESC
		LIMIT -1 OFFSET ?
	`, instanceID, maxCount)
	if err != nil {
		return nil, fmt.Errorf("query old backups: %w", err)
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, fmt.Errorf("scan backup path: %w", err)
		}
		paths = append(paths, path)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Delete old records
	if len(paths) > 0 {
		_, err = d.Exec(`
			DELETE FROM backups
			WHERE instance_id = ? AND id NOT IN (
				SELECT id FROM backups
				WHERE instance_id = ?
				ORDER BY created_at DESC
				LIMIT ?
			)
		`, instanceID, instanceID, maxCount)
		if err != nil {
			return nil, fmt.Errorf("delete old backups: %w", err)
		}
	}

	return paths, nil
}

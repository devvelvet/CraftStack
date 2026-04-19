package store

import (
	"fmt"
	"time"
)

// AuditLog represents a single audit log entry.
type AuditLog struct {
	ID         int       `json:"id"`
	Timestamp  time.Time `json:"timestamp"`
	UserID     *int      `json:"user_id,omitempty"`
	Username   string    `json:"username"`
	Action     string    `json:"action"`      // "create", "update", "delete", "start", "stop", "restart", "kill", "backup", "restore", "connect", "disconnect", "approve", "reject", "role_change"
	TargetType string    `json:"target_type"` // "instance", "network", "user", "node", "mesh", "sync", "backup"
	TargetID   string    `json:"target_id"`
	TargetName string    `json:"target_name"`
	FieldName  string    `json:"field_name"` // specific field changed (for updates)
	OldValue   string    `json:"old_value"`
	NewValue   string    `json:"new_value"`
	Detail     string    `json:"detail"` // human-readable description
	RolledBack bool      `json:"rolled_back"`
	CreatedAt  time.Time `json:"created_at"`
}

// CreateAuditLog inserts a new audit log entry.
func (d *DB) CreateAuditLog(log *AuditLog) error {
	_, err := d.Exec(`
		INSERT INTO audit_logs (user_id, username, action, target_type, target_id, target_name, field_name, old_value, new_value, detail)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, log.UserID, log.Username, log.Action, log.TargetType, log.TargetID, log.TargetName, log.FieldName, log.OldValue, log.NewValue, log.Detail)
	if err != nil {
		return fmt.Errorf("create audit log: %w", err)
	}
	return nil
}

// ListAuditLogs returns audit logs with pagination. limit=0 means all.
func (d *DB) ListAuditLogs(limit, offset int) ([]*AuditLog, error) {
	var query string
	var args []interface{}
	if limit > 0 {
		query = `SELECT id, timestamp, user_id, username, action, target_type, target_id, target_name, field_name, old_value, new_value, detail, rolled_back, created_at FROM audit_logs ORDER BY timestamp DESC LIMIT ? OFFSET ?`
		args = []interface{}{limit, offset}
	} else {
		query = `SELECT id, timestamp, user_id, username, action, target_type, target_id, target_name, field_name, old_value, new_value, detail, rolled_back, created_at FROM audit_logs ORDER BY timestamp DESC`
	}
	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list audit logs: %w", err)
	}
	defer rows.Close()

	var logs []*AuditLog
	for rows.Next() {
		l := &AuditLog{}
		var rolledBack int
		if err := rows.Scan(&l.ID, &l.Timestamp, &l.UserID, &l.Username, &l.Action, &l.TargetType, &l.TargetID, &l.TargetName, &l.FieldName, &l.OldValue, &l.NewValue, &l.Detail, &rolledBack, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan audit log: %w", err)
		}
		l.RolledBack = rolledBack != 0
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

// ListAuditLogsByTarget returns audit logs for a specific target.
func (d *DB) ListAuditLogsByTarget(targetType, targetID string, limit int) ([]*AuditLog, error) {
	rows, err := d.Query(`
		SELECT id, timestamp, user_id, username, action, target_type, target_id, target_name, field_name, old_value, new_value, detail, rolled_back, created_at
		FROM audit_logs WHERE target_type = ? AND target_id = ? ORDER BY timestamp DESC LIMIT ?
	`, targetType, targetID, limit)
	if err != nil {
		return nil, fmt.Errorf("list audit logs by target: %w", err)
	}
	defer rows.Close()

	var logs []*AuditLog
	for rows.Next() {
		l := &AuditLog{}
		var rolledBack int
		if err := rows.Scan(&l.ID, &l.Timestamp, &l.UserID, &l.Username, &l.Action, &l.TargetType, &l.TargetID, &l.TargetName, &l.FieldName, &l.OldValue, &l.NewValue, &l.Detail, &rolledBack, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan audit log: %w", err)
		}
		l.RolledBack = rolledBack != 0
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

// GetAuditLog returns a single audit log entry by ID.
func (d *DB) GetAuditLog(id int) (*AuditLog, error) {
	row := d.QueryRow(`
		SELECT id, timestamp, user_id, username, action, target_type, target_id, target_name, field_name, old_value, new_value, detail, rolled_back, created_at
		FROM audit_logs WHERE id = ?
	`, id)
	l := &AuditLog{}
	var rolledBack int
	err := row.Scan(&l.ID, &l.Timestamp, &l.UserID, &l.Username, &l.Action, &l.TargetType, &l.TargetID, &l.TargetName, &l.FieldName, &l.OldValue, &l.NewValue, &l.Detail, &rolledBack, &l.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get audit log: %w", err)
	}
	l.RolledBack = rolledBack != 0
	return l, nil
}

// MarkAuditLogRolledBack marks an audit log entry as rolled back.
func (d *DB) MarkAuditLogRolledBack(id int) error {
	_, err := d.Exec(`UPDATE audit_logs SET rolled_back = 1 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("mark audit log rolled back: %w", err)
	}
	return nil
}

// CountAuditLogs returns the total number of audit log entries.
func (d *DB) CountAuditLogs() (int, error) {
	var count int
	err := d.QueryRow(`SELECT COUNT(*) FROM audit_logs`).Scan(&count)
	return count, err
}

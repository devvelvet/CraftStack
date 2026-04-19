package store

import (
	"fmt"
	"strings"
	"time"
)

// SyncMapping represents a file synchronization mapping stored in DB.
type SyncMapping struct {
	ID               int       `json:"id"`
	Name             string    `json:"name"`
	Src              string    `json:"src"`     // master  aslocal cache path (itwhen or cache)
	Dest             string    `json:"dest"`    // itwhen: agent work_dir basis relative path
	Targets          string    `json:"targets"` // itwhen: target agent (comma separator)
	Exclude          string    `json:"exclude"` // exclude pattern (comma separator)
	Enabled          bool      `json:"enabled"`
	SourceAgentID    string    `json:"source_agent_id"`    // source agent ID (if empty master  aslocal)
	SourceInstanceID string    `json:"source_instance_id"` // source instance ID
	SourcePath       string    `json:"source_path"`        // source instance my path (e.g.: plugins)
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// IsAgentSource returns true if the source is an agent instance (not master local).
func (m *SyncMapping) IsAgentSource() bool {
	return m.SourceAgentID != "" && m.SourceInstanceID != ""
}

// TargetList returns the targets as a string slice.
func (m *SyncMapping) TargetList() []string {
	if m.Targets == "" || m.Targets == "*" {
		return []string{"*"}
	}
	parts := strings.Split(m.Targets, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	if len(result) == 0 {
		return []string{"*"}
	}
	return result
}

// ExcludeList returns the exclude patterns as a string slice.
func (m *SyncMapping) ExcludeList() []string {
	if m.Exclude == "" {
		return nil
	}
	parts := strings.Split(m.Exclude, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// CreateSyncMapping inserts a new sync mapping.
func (d *DB) CreateSyncMapping(m *SyncMapping) error {
	result, err := d.Exec(`
		INSERT INTO sync_mappings (name, src, dest, targets, exclude, enabled, source_agent_id, source_instance_id, source_path)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, m.Name, m.Src, m.Dest, m.Targets, m.Exclude, m.Enabled, m.SourceAgentID, m.SourceInstanceID, m.SourcePath)
	if err != nil {
		return fmt.Errorf("create sync mapping: %w", err)
	}
	id, _ := result.LastInsertId()
	m.ID = int(id)
	return nil
}

// UpdateSyncMapping updates an existing sync mapping.
func (d *DB) UpdateSyncMapping(m *SyncMapping) error {
	_, err := d.Exec(`
		UPDATE sync_mappings
		SET name = ?, src = ?, dest = ?, targets = ?, exclude = ?, enabled = ?,
		    source_agent_id = ?, source_instance_id = ?, source_path = ?,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, m.Name, m.Src, m.Dest, m.Targets, m.Exclude, m.Enabled,
		m.SourceAgentID, m.SourceInstanceID, m.SourcePath, m.ID)
	if err != nil {
		return fmt.Errorf("update sync mapping: %w", err)
	}
	return nil
}

// DeleteSyncMapping deletes a sync mapping by ID (cascade deletes sync_targets).
func (d *DB) DeleteSyncMapping(id int) error {
	_, err := d.Exec(`DELETE FROM sync_mappings WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete sync mapping: %w", err)
	}
	return nil
}

// GetSyncMapping returns a single sync mapping by ID.
func (d *DB) GetSyncMapping(id int) (*SyncMapping, error) {
	m := &SyncMapping{}
	err := d.QueryRow(`
		SELECT id, name, src, dest, targets, exclude, enabled,
		       source_agent_id, source_instance_id, source_path,
		       created_at, updated_at
		FROM sync_mappings WHERE id = ?
	`, id).Scan(&m.ID, &m.Name, &m.Src, &m.Dest, &m.Targets, &m.Exclude, &m.Enabled,
		&m.SourceAgentID, &m.SourceInstanceID, &m.SourcePath,
		&m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get sync mapping: %w", err)
	}
	return m, nil
}

// ListSyncMappings returns all sync mappings.
func (d *DB) ListSyncMappings() ([]*SyncMapping, error) {
	rows, err := d.Query(`
		SELECT id, name, src, dest, targets, exclude, enabled,
		       source_agent_id, source_instance_id, source_path,
		       created_at, updated_at
		FROM sync_mappings ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("list sync mappings: %w", err)
	}
	defer rows.Close()

	var mappings []*SyncMapping
	for rows.Next() {
		m := &SyncMapping{}
		if err := rows.Scan(&m.ID, &m.Name, &m.Src, &m.Dest, &m.Targets, &m.Exclude, &m.Enabled,
			&m.SourceAgentID, &m.SourceInstanceID, &m.SourcePath,
			&m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan sync mapping: %w", err)
		}
		mappings = append(mappings, m)
	}
	return mappings, rows.Err()
}

// ListEnabledSyncMappings returns only enabled sync mappings.
func (d *DB) ListEnabledSyncMappings() ([]*SyncMapping, error) {
	rows, err := d.Query(`
		SELECT id, name, src, dest, targets, exclude, enabled,
		       source_agent_id, source_instance_id, source_path,
		       created_at, updated_at
		FROM sync_mappings WHERE enabled = 1 ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("list enabled sync mappings: %w", err)
	}
	defer rows.Close()

	var mappings []*SyncMapping
	for rows.Next() {
		m := &SyncMapping{}
		if err := rows.Scan(&m.ID, &m.Name, &m.Src, &m.Dest, &m.Targets, &m.Exclude, &m.Enabled,
			&m.SourceAgentID, &m.SourceInstanceID, &m.SourcePath,
			&m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan sync mapping: %w", err)
		}
		mappings = append(mappings, m)
	}
	return mappings, rows.Err()
}

// --- SyncTarget: mappingper target agent per settings ---

// SyncTarget represents a per-agent target configuration for a sync mapping.
type SyncTarget struct {
	ID         int       `json:"id"`
	MappingID  int       `json:"mapping_id"`
	AgentID    string    `json:"agent_id"`
	InstanceID string    `json:"instance_id"`
	DestPath   string    `json:"dest_path"` // the instance work_dir basis relative path
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
}

// CreateSyncTarget inserts a new sync target.
func (d *DB) CreateSyncTarget(t *SyncTarget) error {
	result, err := d.Exec(`
		INSERT INTO sync_targets (mapping_id, agent_id, instance_id, dest_path, enabled)
		VALUES (?, ?, ?, ?, ?)
	`, t.MappingID, t.AgentID, t.InstanceID, t.DestPath, t.Enabled)
	if err != nil {
		return fmt.Errorf("create sync target: %w", err)
	}
	id, _ := result.LastInsertId()
	t.ID = int(id)
	return nil
}

// UpdateSyncTarget updates a sync target's dest path and enabled state.
func (d *DB) UpdateSyncTarget(t *SyncTarget) error {
	_, err := d.Exec(`
		UPDATE sync_targets SET dest_path = ?, enabled = ? WHERE id = ?
	`, t.DestPath, t.Enabled, t.ID)
	if err != nil {
		return fmt.Errorf("update sync target: %w", err)
	}
	return nil
}

// DeleteSyncTarget deletes a sync target by ID.
func (d *DB) DeleteSyncTarget(id int) error {
	_, err := d.Exec(`DELETE FROM sync_targets WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete sync target: %w", err)
	}
	return nil
}

// ListSyncTargets returns all targets for a mapping.
func (d *DB) ListSyncTargets(mappingID int) ([]*SyncTarget, error) {
	rows, err := d.Query(`
		SELECT id, mapping_id, agent_id, instance_id, dest_path, enabled, created_at
		FROM sync_targets WHERE mapping_id = ? ORDER BY id
	`, mappingID)
	if err != nil {
		return nil, fmt.Errorf("list sync targets: %w", err)
	}
	defer rows.Close()

	var targets []*SyncTarget
	for rows.Next() {
		t := &SyncTarget{}
		if err := rows.Scan(&t.ID, &t.MappingID, &t.AgentID, &t.InstanceID, &t.DestPath, &t.Enabled, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan sync target: %w", err)
		}
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

// ListEnabledSyncTargets returns only enabled targets for a mapping.
func (d *DB) ListEnabledSyncTargets(mappingID int) ([]*SyncTarget, error) {
	rows, err := d.Query(`
		SELECT id, mapping_id, agent_id, instance_id, dest_path, enabled, created_at
		FROM sync_targets WHERE mapping_id = ? AND enabled = 1 ORDER BY id
	`, mappingID)
	if err != nil {
		return nil, fmt.Errorf("list enabled sync targets: %w", err)
	}
	defer rows.Close()

	var targets []*SyncTarget
	for rows.Next() {
		t := &SyncTarget{}
		if err := rows.Scan(&t.ID, &t.MappingID, &t.AgentID, &t.InstanceID, &t.DestPath, &t.Enabled, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan sync target: %w", err)
		}
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

// BulkSetSyncTargets replaces all targets for a mapping with the given list.
// It deletes existing targets and inserts the new ones in a single transaction.
func (d *DB) BulkSetSyncTargets(mappingID int, targets []*SyncTarget) error {
	tx, err := d.Begin()
	if err != nil {
		return fmt.Errorf("bulk set sync targets begin: %w", err)
	}
	defer tx.Rollback()

	// Delete all existing targets for this mapping
	if _, err := tx.Exec(`DELETE FROM sync_targets WHERE mapping_id = ?`, mappingID); err != nil {
		return fmt.Errorf("bulk set sync targets delete: %w", err)
	}

	// Insert new targets
	for _, t := range targets {
		t.MappingID = mappingID
		if t.DestPath == "" {
			t.DestPath = "."
		}
		result, err := tx.Exec(`
			INSERT INTO sync_targets (mapping_id, agent_id, instance_id, dest_path, enabled)
			VALUES (?, ?, ?, ?, ?)
		`, mappingID, t.AgentID, t.InstanceID, t.DestPath, t.Enabled)
		if err != nil {
			return fmt.Errorf("bulk set sync targets insert: %w", err)
		}
		id, _ := result.LastInsertId()
		t.ID = int(id)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("bulk set sync targets commit: %w", err)
	}
	return nil
}

package store

import (
	"fmt"
	"time"
)

// Node represents a registered agent server.
type Node struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Address   string    `json:"address"`
	Status    string    `json:"status"`
	CPUCores  int       `json:"cpu_cores"`
	MemoryMB  int64     `json:"memory_mb"`
	OSInfo    string    `json:"os_info"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// WireGuard mesh network settings
	WGPublicKey  string `json:"wg_public_key"`
	WGPrivateKey string `json:"wg_private_key"`
	WGAddress    string `json:"wg_address"`     // 10.10.0.N/16
	WGEndpoint   string `json:"wg_endpoint"`    // public IP:51820
	WGListenPort int    `json:"wg_listen_port"` // default 51820
	DockerSubnet string `json:"docker_subnet"`  // 172.30.N.0/24
}

// CreateNode inserts a new node into the database.
func (d *DB) CreateNode(n *Node) error {
	_, err := d.Exec(`
		INSERT INTO nodes (id, name, address, status, cpu_cores, memory_mb, os_info)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, n.ID, n.Name, n.Address, n.Status, n.CPUCores, n.MemoryMB, n.OSInfo)
	if err != nil {
		return fmt.Errorf("create node: %w", err)
	}
	return nil
}

// GetNode retrieves a node by ID.
func (d *DB) GetNode(id string) (*Node, error) {
	n := &Node{}
	err := d.QueryRow(`
		SELECT id, name, address, status, cpu_cores, memory_mb, COALESCE(os_info, ''),
		       created_at, updated_at,
		       COALESCE(wg_public_key, ''), COALESCE(wg_private_key, ''),
		       COALESCE(wg_address, ''), COALESCE(wg_endpoint, ''),
		       COALESCE(wg_listen_port, 51820), COALESCE(docker_subnet, '')
		FROM nodes WHERE id = ?
	`, id).Scan(&n.ID, &n.Name, &n.Address, &n.Status, &n.CPUCores, &n.MemoryMB,
		&n.OSInfo, &n.CreatedAt, &n.UpdatedAt,
		&n.WGPublicKey, &n.WGPrivateKey, &n.WGAddress, &n.WGEndpoint,
		&n.WGListenPort, &n.DockerSubnet)
	if err != nil {
		return nil, fmt.Errorf("get node %s: %w", id, err)
	}
	return n, nil
}

// ListNodes returns all registered nodes.
func (d *DB) ListNodes() ([]*Node, error) {
	rows, err := d.Query(`
		SELECT id, name, address, status, cpu_cores, memory_mb, COALESCE(os_info, ''),
		       created_at, updated_at,
		       COALESCE(wg_public_key, ''), COALESCE(wg_private_key, ''),
		       COALESCE(wg_address, ''), COALESCE(wg_endpoint, ''),
		       COALESCE(wg_listen_port, 51820), COALESCE(docker_subnet, '')
		FROM nodes ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	defer rows.Close()

	var nodes []*Node
	for rows.Next() {
		n := &Node{}
		if err := rows.Scan(&n.ID, &n.Name, &n.Address, &n.Status, &n.CPUCores,
			&n.MemoryMB, &n.OSInfo, &n.CreatedAt, &n.UpdatedAt,
			&n.WGPublicKey, &n.WGPrivateKey, &n.WGAddress, &n.WGEndpoint,
			&n.WGListenPort, &n.DockerSubnet); err != nil {
			return nil, fmt.Errorf("scan node: %w", err)
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// UpdateNodeStatus updates the status and timestamp of a node.
func (d *DB) UpdateNodeStatus(id, status string) error {
	_, err := d.Exec(`
		UPDATE nodes SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, status, id)
	if err != nil {
		return fmt.Errorf("update node status: %w", err)
	}
	return nil
}

// UpsertNode inserts or updates a node (used during agent registration).
func (d *DB) UpsertNode(n *Node) error {
	_, err := d.Exec(`
		INSERT INTO nodes (id, name, address, status, cpu_cores, memory_mb, os_info)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			address = excluded.address,
			status = excluded.status,
			cpu_cores = excluded.cpu_cores,
			memory_mb = excluded.memory_mb,
			os_info = excluded.os_info,
			updated_at = CURRENT_TIMESTAMP
	`, n.ID, n.Name, n.Address, n.Status, n.CPUCores, n.MemoryMB, n.OSInfo)
	if err != nil {
		return fmt.Errorf("upsert node: %w", err)
	}
	return nil
}

// ResetAllStatus resets all nodes and instances to offline/stopped on master startup.
// This prevents stale "online"/"running" states from a previous session.
func (d *DB) ResetAllStatus() error {
	if _, err := d.Exec(`UPDATE nodes SET status = 'offline', updated_at = CURRENT_TIMESTAMP`); err != nil {
		return fmt.Errorf("reset node status: %w", err)
	}
	if _, err := d.Exec(`UPDATE instances SET status = 'stopped', pid = NULL, updated_at = CURRENT_TIMESTAMP`); err != nil {
		return fmt.Errorf("reset instance status: %w", err)
	}
	return nil
}

// DeleteNode removes a node by ID.
func (d *DB) DeleteNode(id string) error {
	_, err := d.Exec("DELETE FROM nodes WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete node: %w", err)
	}
	return nil
}

// UpdateNodeWireGuard updates WireGuard mesh settings for a node.
func (d *DB) UpdateNodeWireGuard(id, pubKey, privKey, address, endpoint string, listenPort int, dockerSubnet string) error {
	_, err := d.Exec(`
		UPDATE nodes SET
			wg_public_key = ?, wg_private_key = ?, wg_address = ?,
			wg_endpoint = ?, wg_listen_port = ?, docker_subnet = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, pubKey, privKey, address, endpoint, listenPort, dockerSubnet, id)
	if err != nil {
		return fmt.Errorf("update node wireguard: %w", err)
	}
	return nil
}

// ListNodesWithWireGuard returns nodes that have WireGuard configured.
func (d *DB) ListNodesWithWireGuard() ([]*Node, error) {
	rows, err := d.Query(`
		SELECT id, name, address, status, cpu_cores, memory_mb, COALESCE(os_info, ''),
		       created_at, updated_at,
		       wg_public_key, wg_private_key, wg_address, wg_endpoint,
		       wg_listen_port, docker_subnet
		FROM nodes WHERE wg_public_key != '' ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("list wg nodes: %w", err)
	}
	defer rows.Close()

	var nodes []*Node
	for rows.Next() {
		n := &Node{}
		if err := rows.Scan(&n.ID, &n.Name, &n.Address, &n.Status, &n.CPUCores,
			&n.MemoryMB, &n.OSInfo, &n.CreatedAt, &n.UpdatedAt,
			&n.WGPublicKey, &n.WGPrivateKey, &n.WGAddress, &n.WGEndpoint,
			&n.WGListenPort, &n.DockerSubnet); err != nil {
			return nil, fmt.Errorf("scan wg node: %w", err)
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// NextWGIndex returns the next available WireGuard node index (1-based).
// Used for assigning 10.10.0.N addresses.
func (d *DB) NextWGIndex() (int, error) {
	var maxIdx int
	err := d.QueryRow(`
		SELECT COALESCE(MAX(CAST(REPLACE(REPLACE(wg_address, '10.10.0.', ''), '/16', '') AS INTEGER)), 0)
		FROM nodes WHERE wg_address != ''
	`).Scan(&maxIdx)
	if err != nil {
		return 1, nil
	}
	return maxIdx + 1, nil
}

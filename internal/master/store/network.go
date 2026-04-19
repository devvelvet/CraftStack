package store

import (
	"fmt"
	"time"
)

// Network represents a Docker virtual network managed by CraftStack.
type Network struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Driver    string    `json:"driver"`  // bridge, overlay
	Subnet    string    `json:"subnet"`  // e.g.: "172.20.0.0/16"
	Gateway   string    `json:"gateway"` // e.g.: "172.20.0.1"
	NodeID    string    `json:"node_id"` //  network create agent ID
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// InstanceNetwork represents the mapping between an instance and a Docker network.
type InstanceNetwork struct {
	ID         int       `json:"id"`
	InstanceID string    `json:"instance_id"`
	NetworkID  string    `json:"network_id"`
	Alias      string    `json:"alias"`      // network my DNS alias
	IPAddress  string    `json:"ip_address"` // fixed IP (if empty auto)
	CreatedAt  time.Time `json:"created_at"`
}

// CreateNetwork inserts a new network.
func (d *DB) CreateNetwork(n *Network) error {
	_, err := d.Exec(`
		INSERT INTO networks (id, name, driver, subnet, gateway, node_id)
		VALUES (?, ?, ?, ?, ?, ?)
	`, n.ID, n.Name, n.Driver, n.Subnet, n.Gateway, n.NodeID)
	if err != nil {
		return fmt.Errorf("create network: %w", err)
	}
	return nil
}

// GetNetwork retrieves a network by ID.
func (d *DB) GetNetwork(id string) (*Network, error) {
	row := d.QueryRow(`
		SELECT id, name, driver, subnet, gateway, node_id, created_at, updated_at
		FROM networks WHERE id = ?
	`, id)
	n := &Network{}
	err := row.Scan(&n.ID, &n.Name, &n.Driver, &n.Subnet, &n.Gateway, &n.NodeID, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get network %s: %w", id, err)
	}
	return n, nil
}

// GetNetworkByName retrieves a network by name.
func (d *DB) GetNetworkByName(name string) (*Network, error) {
	row := d.QueryRow(`
		SELECT id, name, driver, subnet, gateway, node_id, created_at, updated_at
		FROM networks WHERE name = ?
	`, name)
	n := &Network{}
	err := row.Scan(&n.ID, &n.Name, &n.Driver, &n.Subnet, &n.Gateway, &n.NodeID, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get network by name %s: %w", name, err)
	}
	return n, nil
}

// ListNetworks returns all networks, optionally filtered by node.
func (d *DB) ListNetworks(nodeID string) ([]*Network, error) {
	var query string
	var args []interface{}

	if nodeID != "" {
		query = `SELECT id, name, driver, subnet, gateway, node_id, created_at, updated_at
		         FROM networks WHERE node_id = ? ORDER BY name`
		args = append(args, nodeID)
	} else {
		query = `SELECT id, name, driver, subnet, gateway, node_id, created_at, updated_at
		         FROM networks ORDER BY name`
	}

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list networks: %w", err)
	}
	defer rows.Close()

	var networks []*Network
	for rows.Next() {
		n := &Network{}
		if err := rows.Scan(&n.ID, &n.Name, &n.Driver, &n.Subnet, &n.Gateway, &n.NodeID, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan network: %w", err)
		}
		networks = append(networks, n)
	}
	return networks, rows.Err()
}

// DeleteNetwork removes a network by ID.
func (d *DB) DeleteNetwork(id string) error {
	_, err := d.Exec("DELETE FROM networks WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete network: %w", err)
	}
	return nil
}

// --- Instance-Network Mapping ---

// AddInstanceToNetwork adds an instance to a network.
func (d *DB) AddInstanceToNetwork(instanceID, networkID, alias, ipAddress string) error {
	_, err := d.Exec(`
		INSERT INTO instance_networks (instance_id, network_id, alias, ip_address)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(instance_id, network_id) DO UPDATE SET
			alias = excluded.alias,
			ip_address = excluded.ip_address
	`, instanceID, networkID, alias, ipAddress)
	if err != nil {
		return fmt.Errorf("add instance to network: %w", err)
	}
	return nil
}

// RemoveInstanceFromNetwork removes an instance from a network.
func (d *DB) RemoveInstanceFromNetwork(instanceID, networkID string) error {
	_, err := d.Exec(`
		DELETE FROM instance_networks WHERE instance_id = ? AND network_id = ?
	`, instanceID, networkID)
	if err != nil {
		return fmt.Errorf("remove instance from network: %w", err)
	}
	return nil
}

// ListInstanceNetworks returns all networks that an instance belongs to.
func (d *DB) ListInstanceNetworks(instanceID string) ([]*InstanceNetwork, error) {
	rows, err := d.Query(`
		SELECT id, instance_id, network_id, alias, ip_address, created_at
		FROM instance_networks WHERE instance_id = ? ORDER BY created_at
	`, instanceID)
	if err != nil {
		return nil, fmt.Errorf("list instance networks: %w", err)
	}
	defer rows.Close()

	var result []*InstanceNetwork
	for rows.Next() {
		in := &InstanceNetwork{}
		if err := rows.Scan(&in.ID, &in.InstanceID, &in.NetworkID, &in.Alias, &in.IPAddress, &in.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan instance network: %w", err)
		}
		result = append(result, in)
	}
	return result, rows.Err()
}

// ListNetworkInstances returns all instances connected to a network.
func (d *DB) ListNetworkInstances(networkID string) ([]*InstanceNetwork, error) {
	rows, err := d.Query(`
		SELECT id, instance_id, network_id, alias, ip_address, created_at
		FROM instance_networks WHERE network_id = ? ORDER BY created_at
	`, networkID)
	if err != nil {
		return nil, fmt.Errorf("list network instances: %w", err)
	}
	defer rows.Close()

	var result []*InstanceNetwork
	for rows.Next() {
		in := &InstanceNetwork{}
		if err := rows.Scan(&in.ID, &in.InstanceID, &in.NetworkID, &in.Alias, &in.IPAddress, &in.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan network instance: %w", err)
		}
		result = append(result, in)
	}
	return result, rows.Err()
}

// CountNetworkInstances returns the number of instances connected to a network.
func (d *DB) CountNetworkInstances(networkID string) (int, error) {
	var count int
	err := d.QueryRow("SELECT COUNT(*) FROM instance_networks WHERE network_id = ?", networkID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count network instances: %w", err)
	}
	return count, nil
}

// --- Mesh Network CRUD ---

// MeshNetwork represents a cross-node WireGuard mesh network.
type MeshNetwork struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	WGCIDR     string    `json:"wg_cidr"`     // WireGuard address range (10.10.0.0/16)
	DockerCIDR string    `json:"docker_cidr"` // Docker subnet range (172.30.0.0/16)
	Domain     string    `json:"domain"`      // DNS main (craftstack.internal)
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// GetDefaultMesh returns the default mesh network.
func (d *DB) GetDefaultMesh() (*MeshNetwork, error) {
	return d.GetMeshNetwork("default")
}

// GetMeshNetwork retrieves a mesh network by ID.
func (d *DB) GetMeshNetwork(id string) (*MeshNetwork, error) {
	m := &MeshNetwork{}
	err := d.QueryRow(`
		SELECT id, name, wg_cidr, docker_cidr, domain, enabled, created_at, updated_at
		FROM mesh_networks WHERE id = ?
	`, id).Scan(&m.ID, &m.Name, &m.WGCIDR, &m.DockerCIDR, &m.Domain, &m.Enabled, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get mesh %s: %w", id, err)
	}
	return m, nil
}

// ListMeshNetworks returns all mesh networks.
func (d *DB) ListMeshNetworks() ([]*MeshNetwork, error) {
	rows, err := d.Query(`
		SELECT id, name, wg_cidr, docker_cidr, domain, enabled, created_at, updated_at
		FROM mesh_networks ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("list mesh networks: %w", err)
	}
	defer rows.Close()

	var meshes []*MeshNetwork
	for rows.Next() {
		m := &MeshNetwork{}
		if err := rows.Scan(&m.ID, &m.Name, &m.WGCIDR, &m.DockerCIDR, &m.Domain, &m.Enabled, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan mesh: %w", err)
		}
		meshes = append(meshes, m)
	}
	return meshes, rows.Err()
}

// --- DNS Record CRUD ---

// DNSRecord represents a DNS entry for cross-node service discovery.
type DNSRecord struct {
	ID         int       `json:"id"`
	MeshID     string    `json:"mesh_id"`
	Name       string    `json:"name"`       // short name (e.g.: "main-db")
	FQDN       string    `json:"fqdn"`       // all main (e.g.: "main-db.craftstack.internal")
	IPAddress  string    `json:"ip_address"` // Docker IP
	InstanceID string    `json:"instance_id"`
	NodeID     string    `json:"node_id"`
	CreatedAt  time.Time `json:"created_at"`
}

// UpsertDNSRecord inserts or updates a DNS record.
func (d *DB) UpsertDNSRecord(r *DNSRecord) error {
	_, err := d.Exec(`
		INSERT INTO dns_records (mesh_id, name, fqdn, ip_address, instance_id, node_id)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(mesh_id, fqdn) DO UPDATE SET
			ip_address = excluded.ip_address,
			instance_id = excluded.instance_id,
			node_id = excluded.node_id
	`, r.MeshID, r.Name, r.FQDN, r.IPAddress, r.InstanceID, r.NodeID)
	if err != nil {
		return fmt.Errorf("upsert dns record: %w", err)
	}
	return nil
}

// DeleteDNSRecord removes a DNS record by instance ID.
func (d *DB) DeleteDNSRecordByInstance(instanceID string) error {
	_, err := d.Exec("DELETE FROM dns_records WHERE instance_id = ?", instanceID)
	if err != nil {
		return fmt.Errorf("delete dns record: %w", err)
	}
	return nil
}

// ListDNSRecords returns all DNS records for a mesh network (or all if meshID is empty).
func (d *DB) ListDNSRecords(meshID string) ([]*DNSRecord, error) {
	var query string
	var args []interface{}
	if meshID != "" {
		query = `SELECT id, mesh_id, name, fqdn, ip_address, instance_id, node_id, created_at
		         FROM dns_records WHERE mesh_id = ? ORDER BY fqdn`
		args = append(args, meshID)
	} else {
		query = `SELECT id, mesh_id, name, fqdn, ip_address, instance_id, node_id, created_at
		         FROM dns_records ORDER BY fqdn`
	}
	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list dns records: %w", err)
	}
	defer rows.Close()

	var records []*DNSRecord
	for rows.Next() {
		r := &DNSRecord{}
		if err := rows.Scan(&r.ID, &r.MeshID, &r.Name, &r.FQDN, &r.IPAddress, &r.InstanceID, &r.NodeID, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan dns record: %w", err)
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

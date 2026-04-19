package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Supported instance types.
const (
	InstanceTypeMinecraft  = "minecraft"
	InstanceTypeMySQL      = "mysql"
	InstanceTypePostgreSQL = "postgresql"
	InstanceTypeMongoDB    = "mongodb"
	InstanceTypeRedis      = "redis"
	InstanceTypeKafka      = "kafka"
)

// ValidInstanceTypes is the set of all supported instance types.
var ValidInstanceTypes = map[string]bool{
	InstanceTypeMinecraft:  true,
	InstanceTypeMySQL:      true,
	InstanceTypePostgreSQL: true,
	InstanceTypeMongoDB:    true,
	InstanceTypeRedis:      true,
	InstanceTypeKafka:      true,
}

// Instance represents a managed service instance on a node.
type Instance struct {
	ID           string    `json:"id"`
	NodeID       string    `json:"node_id"`
	Name         string    `json:"name"`
	Port         int       `json:"port"`
	MemoryMin    string    `json:"memory_min"`
	MemoryMax    string    `json:"memory_max"`
	JavaPath     string    `json:"java_path"`
	ServerJar    string    `json:"server_jar"`
	WorkDir      string    `json:"work_dir"`
	Status       string    `json:"status"`
	PID          *int      `json:"pid,omitempty"`
	RCONPort     *int      `json:"rcon_port,omitempty"`
	RCONPassword *string   `json:"rcon_password,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// settings field (v4 migration)
	AutoStart       bool   `json:"auto_start"`
	AutoRestart     bool   `json:"auto_restart"`
	RestartDelaySec int    `json:"restart_delay_sec"`
	StopCommand     string `json:"stop_command"`
	JVMArgs         string `json:"jvm_args"` // newline separator string
	AcceptEULA      bool   `json:"accept_eula"`

	// instance type (v5 migration)
	InstanceType   string `json:"instance_type"`
	ServiceVersion string `json:"service_version"`

	// MySQL dedicated
	MySQLRootPassword string `json:"mysql_root_password,omitempty"`
	MySQLDataDir      string `json:"mysql_data_dir,omitempty"`
	MySQLExtraArgs    string `json:"mysql_extra_args,omitempty"`

	// PostgreSQL dedicated
	PGPassword  string `json:"pg_password,omitempty"`
	PGDataDir   string `json:"pg_data_dir,omitempty"`
	PGExtraArgs string `json:"pg_extra_args,omitempty"`

	// MongoDB dedicated
	MongoAdminUser     string `json:"mongo_admin_user,omitempty"`
	MongoAdminPassword string `json:"mongo_admin_password,omitempty"`
	MongoDataDir       string `json:"mongo_data_dir,omitempty"`
	MongoExtraArgs     string `json:"mongo_extra_args,omitempty"`

	// Redis dedicated
	RedisPassword  string `json:"redis_password,omitempty"`
	RedisDataDir   string `json:"redis_data_dir,omitempty"`
	RedisExtraArgs string `json:"redis_extra_args,omitempty"`

	// Kafka dedicated
	KafkaBrokerID  int    `json:"kafka_broker_id,omitempty"`
	KafkaDataDir   string `json:"kafka_data_dir,omitempty"`
	KafkaExtraArgs string `json:"kafka_extra_args,omitempty"`

	// backup schedule (v7 migration)
	BackupEnabled  bool       `json:"backup_enabled"`
	BackupCron     string     `json:"backup_cron"`
	BackupMaxCount int        `json:"backup_max_count"`
	BackupLastAt   *time.Time `json:"backup_last_at,omitempty"`

	// Java version + Docker customization (v8 migration)
	JavaVersion      string `json:"java_version"`      // "17", "21", "25" (if empty default 21)
	CustomDockerfile string `json:"custom_dockerfile"` // custom Dockerfile content
	CustomCompose    string `json:"custom_compose"`    // custom docker-compose.yml content

	// Docker resource limit (v11 migration)
	DockerMemory string `json:"docker_memory"` // Docker --memory (e.g.: "2G", if empty, 1.5x of memory_max)
	DockerCPUs   string `json:"docker_cpus"`   // Docker --cpus (e.g.: "2.0", if empty, no limit)
}

// JVMArgsList returns JVM args as a slice.
func (i *Instance) JVMArgsList() []string {
	if i.JVMArgs == "" {
		return nil
	}
	var args []string
	for _, line := range strings.Split(i.JVMArgs, "\n") {
		arg := strings.TrimSpace(line)
		if arg != "" {
			args = append(args, arg)
		}
	}
	return args
}

// SetJVMArgsList sets JVM args from a slice.
func (i *Instance) SetJVMArgsList(args []string) {
	i.JVMArgs = strings.Join(args, "\n")
}

// IsMinecraft returns true if this is a Minecraft instance.
func (i *Instance) IsMinecraft() bool {
	return i.InstanceType == "" || i.InstanceType == InstanceTypeMinecraft
}

// IsJavaBased returns true if this instance type requires a Java runtime.
func (i *Instance) IsJavaBased() bool {
	return i.IsMinecraft() || i.InstanceType == InstanceTypeKafka
}

// instance scan columns (shared across queries)
const instanceSelectCols = `id, node_id, name, port, memory_min, memory_max, java_path,
	server_jar, work_dir, status, pid, rcon_port, rcon_password,
	auto_start, auto_restart, restart_delay_sec, stop_command, jvm_args, accept_eula,
	instance_type, service_version,
	mysql_root_password, mysql_data_dir, mysql_extra_args,
	pg_password, pg_data_dir, pg_extra_args,
	mongo_admin_user, mongo_admin_password, mongo_data_dir, mongo_extra_args,
	redis_password, redis_data_dir, redis_extra_args,
	kafka_broker_id, kafka_data_dir, kafka_extra_args,
	backup_enabled, backup_cron, backup_max_count, backup_last_at,
	java_version, custom_dockerfile, custom_compose,
	docker_memory, docker_cpus,
	created_at, updated_at`

func scanInstance(scanner interface{ Scan(dest ...any) error }) (*Instance, error) {
	inst := &Instance{}
	err := scanner.Scan(
		&inst.ID, &inst.NodeID, &inst.Name, &inst.Port,
		&inst.MemoryMin, &inst.MemoryMax, &inst.JavaPath,
		&inst.ServerJar, &inst.WorkDir, &inst.Status,
		&inst.PID, &inst.RCONPort, &inst.RCONPassword,
		&inst.AutoStart, &inst.AutoRestart, &inst.RestartDelaySec,
		&inst.StopCommand, &inst.JVMArgs, &inst.AcceptEULA,
		&inst.InstanceType, &inst.ServiceVersion,
		&inst.MySQLRootPassword, &inst.MySQLDataDir, &inst.MySQLExtraArgs,
		&inst.PGPassword, &inst.PGDataDir, &inst.PGExtraArgs,
		&inst.MongoAdminUser, &inst.MongoAdminPassword, &inst.MongoDataDir, &inst.MongoExtraArgs,
		&inst.RedisPassword, &inst.RedisDataDir, &inst.RedisExtraArgs,
		&inst.KafkaBrokerID, &inst.KafkaDataDir, &inst.KafkaExtraArgs,
		&inst.BackupEnabled, &inst.BackupCron, &inst.BackupMaxCount, &inst.BackupLastAt,
		&inst.JavaVersion, &inst.CustomDockerfile, &inst.CustomCompose,
		&inst.DockerMemory, &inst.DockerCPUs,
		&inst.CreatedAt, &inst.UpdatedAt,
	)
	return inst, err
}

// CreateInstance inserts a new instance.
func (d *DB) CreateInstance(inst *Instance) error {
	// Default instance type
	if inst.InstanceType == "" {
		inst.InstanceType = InstanceTypeMinecraft
	}
	_, err := d.Exec(`
		INSERT INTO instances (id, node_id, name, port, memory_min, memory_max,
		                       java_path, server_jar, work_dir, status, rcon_port, rcon_password,
		                       auto_start, auto_restart, restart_delay_sec, stop_command, jvm_args, accept_eula,
		                       instance_type, service_version,
		                       mysql_root_password, mysql_data_dir, mysql_extra_args,
		                       pg_password, pg_data_dir, pg_extra_args,
		                       mongo_admin_user, mongo_admin_password, mongo_data_dir, mongo_extra_args,
		                       redis_password, redis_data_dir, redis_extra_args,
		                       kafka_broker_id, kafka_data_dir, kafka_extra_args,
		                       backup_enabled, backup_cron, backup_max_count,
		                       java_version, custom_dockerfile, custom_compose,
		                       docker_memory, docker_cpus)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, inst.ID, inst.NodeID, inst.Name, inst.Port, inst.MemoryMin, inst.MemoryMax,
		inst.JavaPath, inst.ServerJar, inst.WorkDir, inst.Status, inst.RCONPort, inst.RCONPassword,
		inst.AutoStart, inst.AutoRestart, inst.RestartDelaySec, inst.StopCommand, inst.JVMArgs, inst.AcceptEULA,
		inst.InstanceType, inst.ServiceVersion,
		inst.MySQLRootPassword, inst.MySQLDataDir, inst.MySQLExtraArgs,
		inst.PGPassword, inst.PGDataDir, inst.PGExtraArgs,
		inst.MongoAdminUser, inst.MongoAdminPassword, inst.MongoDataDir, inst.MongoExtraArgs,
		inst.RedisPassword, inst.RedisDataDir, inst.RedisExtraArgs,
		inst.KafkaBrokerID, inst.KafkaDataDir, inst.KafkaExtraArgs,
		inst.BackupEnabled, inst.BackupCron, inst.BackupMaxCount,
		inst.JavaVersion, inst.CustomDockerfile, inst.CustomCompose,
		inst.DockerMemory, inst.DockerCPUs)
	if err != nil {
		return fmt.Errorf("create instance: %w", err)
	}
	return nil
}

// GetInstance retrieves an instance by ID.
func (d *DB) GetInstance(id string) (*Instance, error) {
	row := d.QueryRow(`SELECT `+instanceSelectCols+` FROM instances WHERE id = ?`, id)
	inst, err := scanInstance(row)
	if err != nil {
		return nil, fmt.Errorf("get instance %s: %w", id, err)
	}
	return inst, nil
}

// ListInstances returns all instances, optionally filtered by node.
func (d *DB) ListInstances(nodeID string) ([]*Instance, error) {
	var rows *sql.Rows
	var err error

	if nodeID != "" {
		rows, err = d.Query(`SELECT `+instanceSelectCols+` FROM instances WHERE node_id = ? ORDER BY name`, nodeID)
	} else {
		rows, err = d.Query(`SELECT ` + instanceSelectCols + ` FROM instances ORDER BY name`)
	}
	if err != nil {
		return nil, fmt.Errorf("list instances: %w", err)
	}
	defer rows.Close()

	var instances []*Instance
	for rows.Next() {
		inst, err := scanInstance(rows)
		if err != nil {
			return nil, fmt.Errorf("scan instance: %w", err)
		}
		instances = append(instances, inst)
	}
	return instances, rows.Err()
}

// ListInstancesByType returns instances filtered by type.
func (d *DB) ListInstancesByType(instanceType string) ([]*Instance, error) {
	rows, err := d.Query(`SELECT `+instanceSelectCols+` FROM instances WHERE instance_type = ? ORDER BY name`, instanceType)
	if err != nil {
		return nil, fmt.Errorf("list instances by type: %w", err)
	}
	defer rows.Close()

	var instances []*Instance
	for rows.Next() {
		inst, err := scanInstance(rows)
		if err != nil {
			return nil, fmt.Errorf("scan instance: %w", err)
		}
		instances = append(instances, inst)
	}
	return instances, rows.Err()
}

// UpdateInstanceStatus updates instance status and PID.
func (d *DB) UpdateInstanceStatus(id, status string, pid *int) error {
	_, err := d.Exec(`
		UPDATE instances SET status = ?, pid = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, status, pid, id)
	if err != nil {
		return fmt.Errorf("update instance status: %w", err)
	}
	return nil
}

// UpsertInstance inserts or updates an instance.
// On conflict, only status is updated (config fields are NOT overwritten by heartbeat).
func (d *DB) UpsertInstance(inst *Instance) error {
	if inst.InstanceType == "" {
		inst.InstanceType = InstanceTypeMinecraft
	}
	_, err := d.Exec(`
		INSERT INTO instances (id, node_id, name, port, memory_min, memory_max,
		                       java_path, server_jar, work_dir, status,
		                       auto_start, auto_restart, restart_delay_sec, stop_command, jvm_args, accept_eula,
		                       instance_type, service_version,
		                       mysql_root_password, mysql_data_dir, mysql_extra_args,
		                       pg_password, pg_data_dir, pg_extra_args,
		                       mongo_admin_user, mongo_admin_password, mongo_data_dir, mongo_extra_args,
		                       redis_password, redis_data_dir, redis_extra_args,
		                       kafka_broker_id, kafka_data_dir, kafka_extra_args,
		                       backup_enabled, backup_cron, backup_max_count,
		                       java_version, custom_dockerfile, custom_compose,
		                       docker_memory, docker_cpus)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			status = excluded.status,
			updated_at = CURRENT_TIMESTAMP
	`, inst.ID, inst.NodeID, inst.Name, inst.Port, inst.MemoryMin, inst.MemoryMax,
		inst.JavaPath, inst.ServerJar, inst.WorkDir, inst.Status,
		inst.AutoStart, inst.AutoRestart, inst.RestartDelaySec, inst.StopCommand, inst.JVMArgs, inst.AcceptEULA,
		inst.InstanceType, inst.ServiceVersion,
		inst.MySQLRootPassword, inst.MySQLDataDir, inst.MySQLExtraArgs,
		inst.PGPassword, inst.PGDataDir, inst.PGExtraArgs,
		inst.MongoAdminUser, inst.MongoAdminPassword, inst.MongoDataDir, inst.MongoExtraArgs,
		inst.RedisPassword, inst.RedisDataDir, inst.RedisExtraArgs,
		inst.KafkaBrokerID, inst.KafkaDataDir, inst.KafkaExtraArgs,
		inst.BackupEnabled, inst.BackupCron, inst.BackupMaxCount,
		inst.JavaVersion, inst.CustomDockerfile, inst.CustomCompose,
		inst.DockerMemory, inst.DockerCPUs)
	if err != nil {
		return fmt.Errorf("upsert instance: %w", err)
	}
	return nil
}

// UpdateInstance updates all config fields of an instance (used by web UI edit).
func (d *DB) UpdateInstance(inst *Instance) error {
	_, err := d.Exec(`
		UPDATE instances SET
			name = ?, port = ?, memory_min = ?, memory_max = ?,
			java_path = ?, server_jar = ?, work_dir = ?,
			auto_start = ?, auto_restart = ?, restart_delay_sec = ?,
			stop_command = ?, jvm_args = ?, accept_eula = ?,
			instance_type = ?, service_version = ?,
			mysql_root_password = ?, mysql_data_dir = ?, mysql_extra_args = ?,
			pg_password = ?, pg_data_dir = ?, pg_extra_args = ?,
			mongo_admin_user = ?, mongo_admin_password = ?, mongo_data_dir = ?, mongo_extra_args = ?,
			redis_password = ?, redis_data_dir = ?, redis_extra_args = ?,
			kafka_broker_id = ?, kafka_data_dir = ?, kafka_extra_args = ?,
			backup_enabled = ?, backup_cron = ?, backup_max_count = ?,
			java_version = ?, custom_dockerfile = ?, custom_compose = ?,
			docker_memory = ?, docker_cpus = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, inst.Name, inst.Port, inst.MemoryMin, inst.MemoryMax,
		inst.JavaPath, inst.ServerJar, inst.WorkDir,
		inst.AutoStart, inst.AutoRestart, inst.RestartDelaySec,
		inst.StopCommand, inst.JVMArgs, inst.AcceptEULA,
		inst.InstanceType, inst.ServiceVersion,
		inst.MySQLRootPassword, inst.MySQLDataDir, inst.MySQLExtraArgs,
		inst.PGPassword, inst.PGDataDir, inst.PGExtraArgs,
		inst.MongoAdminUser, inst.MongoAdminPassword, inst.MongoDataDir, inst.MongoExtraArgs,
		inst.RedisPassword, inst.RedisDataDir, inst.RedisExtraArgs,
		inst.KafkaBrokerID, inst.KafkaDataDir, inst.KafkaExtraArgs,
		inst.BackupEnabled, inst.BackupCron, inst.BackupMaxCount,
		inst.JavaVersion, inst.CustomDockerfile, inst.CustomCompose,
		inst.DockerMemory, inst.DockerCPUs,
		inst.ID)
	if err != nil {
		return fmt.Errorf("update instance: %w", err)
	}
	return nil
}

// UpdateBackupLastAt updates the backup_last_at timestamp for an instance.
func (d *DB) UpdateBackupLastAt(id string, t time.Time) error {
	_, err := d.Exec(`UPDATE instances SET backup_last_at = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, t, id)
	if err != nil {
		return fmt.Errorf("update backup_last_at: %w", err)
	}
	return nil
}

// ListBackupEnabledInstances returns instances with backup_enabled = true.
func (d *DB) ListBackupEnabledInstances() ([]*Instance, error) {
	rows, err := d.Query(`SELECT ` + instanceSelectCols + ` FROM instances WHERE backup_enabled = 1 ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list backup-enabled instances: %w", err)
	}
	defer rows.Close()

	var instances []*Instance
	for rows.Next() {
		inst, err := scanInstance(rows)
		if err != nil {
			return nil, fmt.Errorf("scan instance: %w", err)
		}
		instances = append(instances, inst)
	}
	return instances, rows.Err()
}

// DeleteInstance removes an instance by ID.
func (d *DB) DeleteInstance(id string) error {
	_, err := d.Exec("DELETE FROM instances WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete instance: %w", err)
	}
	return nil
}

package docker

import (
	"fmt"
	"path/filepath"
	"runtime"
)

// ServiceImageConfig holds the default Docker image and configuration for a service type.
type ServiceImageConfig struct {
	Image         string            // Docker image (e.g.: "mysql:8.0")
	DefaultPort   int               // container internal default port
	DataDir       string            // container my data directory (volume mount target)
	ConfigDir     string            // container my settings directory
	Env           map[string]string // default environment variable
	Cmd           []string          // default command override
	StopSignal    bool              // true if docker stop as shutdown (default), false if exec command as shutdown
	StopCommand   []string          // StopSignal=falseday when use shutdown command
	HealthCheck   []string          // healthcheck command
	ReadyLogMatch string            // "ready" log match string for ready detection
}

// DefaultImages returns the default Docker image configuration for each supported service type.
func DefaultImages() map[string]ServiceImageConfig {
	return map[string]ServiceImageConfig{
		"minecraft": {
			Image:       "eclipse-temurin:21-jre",
			DefaultPort: 25565,
			DataDir:     "/server",
			Env: map[string]string{
				"JAVA_TOOL_OPTIONS": "-Xms512M -Xmx1024M",
			},
			Cmd:           []string{"java", "-jar", "server.jar", "nogui"},
			StopSignal:    false,
			StopCommand:   []string{},
			ReadyLogMatch: "Done",
		},
		"mysql": {
			Image:       "mysql:8.0",
			DefaultPort: 3306,
			DataDir:     "/var/lib/mysql",
			ConfigDir:   "/etc/mysql/conf.d",
			Env: map[string]string{
				"MYSQL_ROOT_PASSWORD": "craftstack",
			},
			StopSignal:    true,
			ReadyLogMatch: "ready for connections",
		},
		"postgresql": {
			Image:       "postgres:16",
			DefaultPort: 5432,
			DataDir:     "/var/lib/postgresql/data",
			Env: map[string]string{
				"POSTGRES_PASSWORD": "craftstack",
			},
			StopSignal:    true,
			ReadyLogMatch: "ready to accept connections",
		},
		"mongodb": {
			Image:       "mongo:7",
			DefaultPort: 27017,
			DataDir:     "/data/db",
			Env:         map[string]string{},
			StopSignal:  true,
		},
		"redis": {
			Image:       "redis:7",
			DefaultPort: 6379,
			DataDir:     "/data",
			Env:         map[string]string{},
			StopSignal:  true,
		},
		"kafka": {
			Image:       "apache/kafka:3.7.0",
			DefaultPort: 9092,
			DataDir:     "/var/lib/kafka/data",
			Env: map[string]string{
				"KAFKA_NODE_ID":                                  "1",
				"KAFKA_PROCESS_ROLES":                            "broker,controller",
				"KAFKA_LISTENERS":                                "PLAINTEXT://:9092,CONTROLLER://:9093",
				"KAFKA_CONTROLLER_QUORUM_VOTERS":                 "1@localhost:9093",
				"KAFKA_CONTROLLER_LISTENER_NAMES":                "CONTROLLER",
				"KAFKA_INTER_BROKER_LISTENER_NAME":               "PLAINTEXT",
				"KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR":         "1",
				"KAFKA_TRANSACTION_STATE_LOG_REPLICATION_FACTOR": "1",
				"KAFKA_TRANSACTION_STATE_LOG_MIN_ISR":            "1",
				"KAFKA_LOG_DIRS":                                 "/var/lib/kafka/data",
				"CLUSTER_ID":                                     "MkU3OEVBNTcwNTJENDM2Qk",
			},
			StopSignal: true,
		},
	}
}

// JavaVersionImages maps Java version strings to Eclipse Temurin Docker image tags.
var JavaVersionImages = map[string]string{
	"17": "eclipse-temurin:17-jre",
	"21": "eclipse-temurin:21-jre",
	"25": "eclipse-temurin:25-jre",
}

// ValidJavaVersions is the list of supported Java versions.
var ValidJavaVersions = []string{"17", "21", "25"}

// GetImageConfig returns the Docker image configuration for a service type.
// If version is specified, it overrides the default image tag.
// javaVersion applies only to Java-based services (minecraft, kafka).
func GetImageConfig(serviceType, version, javaVersion string) (ServiceImageConfig, error) {
	images := DefaultImages()
	cfg, ok := images[serviceType]
	if !ok {
		return ServiceImageConfig{}, fmt.Errorf("support not service type: %s", serviceType)
	}

	// Java based type: Java version as image decide
	if serviceType == "minecraft" && javaVersion != "" {
		if img, ok := JavaVersionImages[javaVersion]; ok {
			cfg.Image = img
		}
	}

	// override image tag when version specified (middleware type)
	if version != "" && serviceType != "minecraft" {
		base := cfg.Image
		for i := len(base) - 1; i >= 0; i-- {
			if base[i] == ':' {
				base = base[:i]
				break
			}
		}
		cfg.Image = base + ":" + version
	}

	return cfg, nil
}

// BuildContainerConfig creates a ContainerConfig for a service instance.
func BuildContainerConfig(
	instanceName string,
	serviceType string,
	serviceVersion string,
	hostPort int,
	hostDataDir string,
	envOverrides map[string]string,
	dockerMemory string,
	dockerCPUs string,
	javaVersion string,
	networkName string,
) (ContainerConfig, error) {
	imgCfg, err := GetImageConfig(serviceType, serviceVersion, javaVersion)
	if err != nil {
		return ContainerConfig{}, err
	}

	// container name: craftstack-{instancename}
	containerName := fmt.Sprintf("craftstack-%s", instanceName)

	// host port default
	if hostPort == 0 {
		hostPort = imgCfg.DefaultPort
	}

	// host data directory absolute path as convert
	absDataDir, err := filepath.Abs(hostDataDir)
	if err != nil {
		return ContainerConfig{}, fmt.Errorf("data directory absolute path convert failed: %w", err)
	}

	// Windows from Docker volume mount path convert
	// C:\path → /c/path (Docker Toolbox) or as-is as (Docker Desktop)
	mountPath := absDataDir
	if runtime.GOOS == "windows" {
		// Docker Desktop Windows path as-is as usable
		mountPath = absDataDir
	}

	// port mapping
	ports := map[int]int{
		hostPort: imgCfg.DefaultPort,
	}

	// volume mapping: host data directory → container data directory
	volumes := map[string]string{
		mountPath: imgCfg.DataDir,
	}

	// environment variable: default + override
	env := make(map[string]string)
	for k, v := range imgCfg.Env {
		env[k] = v
	}
	for k, v := range envOverrides {
		env[k] = v
	}

	// special handling for Minecraft type
	var cmd []string
	var workDir string
	if serviceType == "minecraft" {
		workDir = imgCfg.DataDir
		cmd = imgCfg.Cmd
		// JAVA_TOOL_OPTIONS は caller (client.go) の envOverrides で設定済み
	}

	// network default
	if networkName == "" {
		networkName = "craftstack-default"
	}

	return ContainerConfig{
		Name:        containerName,
		Image:       imgCfg.Image,
		Ports:       ports,
		Volumes:     volumes,
		Env:         env,
		Cmd:         cmd,
		RestartMode: "unless-stopped",
		Memory:      dockerMemory,
		CPUs:        dockerCPUs,
		WorkDir:     workDir,
		Network:     networkName,
	}, nil
}

package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	pb "craftstack/gen/proto/craftstack"
	"craftstack/internal/agent/docker"
	"craftstack/internal/agent/process"
	"craftstack/internal/common"
)

// syncInstances synchronizes in-memory instances with the master DB list.
// Adds new instances, skips already-registered ones.
// All instance types (including Minecraft) are run as Docker containers.
func (a *Agent) syncInstances(configs []*pb.InstanceConfig) {
	a.syncMu.Lock()
	defer a.syncMu.Unlock()

	a.log.Info("instance sync start", "count", len(configs))

	for _, cfg := range configs {
		instID := cfg.InstanceId

		a.mu.RLock()
		_, exists := a.instances[instID]
		a.mu.RUnlock()

		if exists {
			// existing registered instance - config change detection to be implemented later
			continue
		}

		// instance type decide
		instType := cfg.InstanceType
		if instType == "" {
			instType = "minecraft"
		}

		// new instance register
		def := &common.InstanceDef{
			Type:             instType,
			Name:             cfg.Name,
			Port:             int(cfg.Port),
			WorkDir:          cfg.WorkDir,
			ServerJar:        cfg.ServerJar,
			MemoryMin:        cfg.MemoryMin,
			MemoryMax:        cfg.MemoryMax,
			JavaPath:         cfg.JavaPath,
			AutoStart:        cfg.AutoStart,
			AutoRestart:      cfg.AutoRestart,
			RestartDelaySec:  int(cfg.RestartDelaySec),
			StopCommand:      cfg.StopCommand,
			JVMArgs:          cfg.JvmArgs,
			AcceptEULA:       cfg.AcceptEula,
			ServiceVersion:   cfg.ServiceVersion,
			JavaVersion:      cfg.JavaVersion,
			CustomDockerfile: cfg.CustomDockerfile,
			CustomCompose:    cfg.CustomCompose,
			NetworkName:      cfg.NetworkName,
			DockerMemory:     cfg.DockerMemory,
			DockerCPUs:       cfg.DockerCpus,
		}

		// default apply
		if def.ServerJar == "" {
			def.ServerJar = "server.jar"
		}
		if def.MemoryMin == "" {
			def.MemoryMin = "512M"
		}
		if def.MemoryMax == "" {
			def.MemoryMax = "1024M"
		}
		if def.JavaPath == "" {
			def.JavaPath = a.cfg.Java.Path
		}
		if def.StopCommand == "" {
			def.StopCommand = "stop"
		}
		if def.RestartDelaySec <= 0 {
			def.RestartDelaySec = 10
		}
		if def.Port == 0 {
			def.Port = 25565
		}

		// work_dir create (Docker volume mount target)
		if err := os.MkdirAll(def.WorkDir, 0755); err != nil {
			a.log.Error("work directory create failed", "name", def.Name, "error", err)
			continue
		}

		// Minecraft type: EULA + server.properties auto create (volume not x)
		if instType == "minecraft" && def.AcceptEULA {
			eulaPath := filepath.Join(def.WorkDir, "eula.txt")
			if _, err := os.Stat(eulaPath); os.IsNotExist(err) {
				content := "# CraftStack auto create\n# https://aka.ms/MinecraftEULA\neula=true\n"
				os.WriteFile(eulaPath, []byte(content), 0644)
			}
		}
		if instType == "minecraft" {
			propsPath := filepath.Join(def.WorkDir, "server.properties")
			if _, err := os.Stat(propsPath); os.IsNotExist(err) {
				props := fmt.Sprintf("# CraftStack auto create — additional settings auto-added after server start\nserver-port=%d\n", def.Port)
				os.WriteFile(propsPath, []byte(props), 0644)
			}
		}

		// Docker create container/connect (5min timeout)
		a.log.Info("Docker container ready during", "name", def.Name, "type", instType, "id", instID)
		syncCtx, syncCancel := context.WithTimeout(a.ctx, 5*time.Minute)
		proc, err := a.ensureDockerContainerCtx(syncCtx, instID, def)
		syncCancel()
		if err != nil {
			a.log.Error("Docker container ready failed", "name", def.Name, "error", err)
			continue
		}

		if err := a.AddInstance(proc); err != nil {
			a.log.Error("instance register failed", "name", def.Name, "error", err)
			continue
		}

		a.mu.Lock()
		a.defs[instID] = def
		a.mu.Unlock()

		a.log.Info("master DB from instance sync", "id", instID, "name", def.Name, "type", instType)

		// auto-start (already runningif skip)
		if def.AutoStart {
			if proc.State() == process.StateRunning || proc.State() == process.StateStarting {
				a.log.Info("instance already running — auto-start skipped", "name", def.Name, "type", instType)
				continue
			}
			if instType == "minecraft" {
				jarPath := filepath.Join(def.WorkDir, def.ServerJar)
				if _, err := os.Stat(jarPath); os.IsNotExist(err) {
					a.log.Warn("no JAR file, skipping auto-start",
						"name", def.Name, "jar_path", jarPath)
					continue
				}
			}
			a.log.Info("instance auto-start", "name", def.Name, "type", instType)
			if err := a.ControlInstance(instID, "start"); err != nil {
				a.log.Error("instance auto-start failed", "name", def.Name, "error", err)
			}
		}
	}

	a.mu.RLock()
	total := len(a.instances)
	a.mu.RUnlock()
	a.log.Info("instance sync complete", "total_instances", total)
}

// ensureDockerContainer ensures a Docker container exists for the instance.
// If the container already exists, returns a DockerProcess pointing to it.
// If not, pulls the image and creates the container.
// Also ensures Docker daemon is running before proceeding.
func (a *Agent) ensureDockerContainer(instID string, def *common.InstanceDef) (process.Process, error) {
	return a.ensureDockerContainerCtx(a.ctx, instID, def)
}

func (a *Agent) ensureDockerContainerCtx(ctx context.Context, instID string, def *common.InstanceDef) (process.Process, error) {
	// Docker daemon execute check — not run if present auto-start attempt
	if !a.dockerMgr.IsRunning() {
		a.log.Warn("Docker daemon execute without . start attempt during...")
		if err := docker.EnsureDocker(ctx, a.log); err != nil {
			return nil, fmt.Errorf("Docker daemon startcannot: %w", err)
		}
	}

	containerName := fmt.Sprintf("craftstack-%s", def.Name)

	// environment variable override configuration
	envOverrides := make(map[string]string)

	switch def.Type {
	case "minecraft":
		envOverrides["JAVA_TOOL_OPTIONS"] = fmt.Sprintf("-Xms%s -Xmx%s", def.MemoryMin, def.MemoryMax)
	case "mysql":
		envOverrides["MYSQL_ROOT_PASSWORD"] = "craftstack"
	case "postgresql":
		envOverrides["POSTGRES_PASSWORD"] = "craftstack"
	}

	// calculate Docker memory limit: DockerMemory settings if present use, if not MemoryMax 1.5x
	effectiveDockerMemory := def.DockerMemory
	if effectiveDockerMemory == "" && def.MemoryMax != "" {
		effectiveDockerMemory = computeDockerMemory(def.MemoryMax)
	}

	// Docker container settings build
	containerCfg, err := docker.BuildContainerConfig(
		def.Name,
		def.Type,
		def.ServiceVersion,
		def.Port,
		def.WorkDir,
		envOverrides,
		effectiveDockerMemory,
		def.DockerCPUs,
		def.JavaVersion,
		def.NetworkName,
	)
	if err != nil {
		return nil, fmt.Errorf("Docker settings build failed: %w", err)
	}

	// network specify if present not create (already existing ignore)
	if containerCfg.Network != "" {
		a.log.Info("Docker network check/create", "network", containerCfg.Network)
		a.dockerMgr.NetworkCreate(ctx, containerCfg.Network, "bridge", "", "")
	}

	// cross node DNS injection: WireGuard DNS server enable if present --dns settings
	if a.wgDNSListenIP != "" {
		containerCfg.DNS = append(containerCfg.DNS, a.wgDNSListenIP)
	}

	// docker-compose mode: compose file if present compose up -d execute
	if def.CustomCompose != "" {
		composePath := filepath.Join(def.WorkDir, "docker-compose.yml")
		if err := os.WriteFile(composePath, []byte(def.CustomCompose), 0644); err != nil {
			return nil, fmt.Errorf("docker-compose.yml save failed: %w", err)
		}
		a.log.Info("docker-compose.yml save complete", "path", composePath)

		if err := a.dockerMgr.ComposeUp(ctx, def.WorkDir); err != nil {
			return nil, fmt.Errorf("docker compose up failed: %w", err)
		}
	}

	// custom Dockerfile mode: Dockerfile save → docker build → replace image name
	if def.CustomDockerfile != "" {
		dockerfilePath := filepath.Join(def.WorkDir, "Dockerfile")
		if err := os.WriteFile(dockerfilePath, []byte(def.CustomDockerfile), 0644); err != nil {
			return nil, fmt.Errorf("Dockerfile save failed: %w", err)
		}
		customImage := fmt.Sprintf("craftstack-custom-%s:latest", def.Name)
		a.log.Info("custom Docker image build", "image", customImage)
		if err := a.dockerMgr.BuildImage(ctx, def.WorkDir, customImage); err != nil {
			return nil, fmt.Errorf("Docker image build failed: %w", err)
		}
		containerCfg.Image = customImage
	}

	// container already that has check
	a.log.Info("Docker container existing check during", "container", containerName)
	if a.dockerMgr.ContainerExists(ctx, containerName) {
		a.log.Info("existing Docker container found", "container", containerName)
		// network recovery: existing container specify network connect that has checkand, if absent connect
		if containerCfg.Network != "" {
			if err := a.dockerMgr.NetworkConnect(ctx, containerCfg.Network, containerName, def.Name, ""); err != nil {
				a.log.Warn("existing container network recovery failed (ignore)", "network", containerCfg.Network, "container", containerName, "error", err)
			}
		}
	} else {
		a.log.Info("Docker container none — new as create", "container", containerName)
		// custom Dockerfile pull image only if not custom
		if def.CustomDockerfile == "" {
			a.log.Info("Docker image pool", "image", containerCfg.Image)
			if err := a.dockerMgr.PullImage(ctx, containerCfg.Image); err != nil {
				return nil, fmt.Errorf("image pool failed: %w", err)
			}
		}

		// create container
		if _, err := a.dockerMgr.CreateContainer(ctx, containerCfg); err != nil {
			return nil, fmt.Errorf("create container failed: %w", err)
		}
	}

	// DockerProcess create
	dockerProc := process.NewDocker(process.DockerConfig{
		ID:            instID,
		Name:          def.Name,
		Type:          def.Type,
		ContainerName: containerName,
		DockerPath:    a.dockerMgr.DockerPath(),
	}, a.log)

	// already runningin containerif state sync
	a.log.Info("Docker container state check during", "container", containerName)
	dockerProc.RefreshState()
	a.log.Info("Docker container ready complete", "container", containerName, "state", string(dockerProc.State()))

	return dockerProc, nil
}

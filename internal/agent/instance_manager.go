package agent

import (
	"fmt"
	"os"
	"path/filepath"

	"craftstack/internal/agent/backup"
	"craftstack/internal/agent/process"
	"craftstack/internal/common"
)

// AddInstance registers a new managed instance with its Process implementation.
func (a *Agent) AddInstance(proc process.Process) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, exists := a.instances[proc.ID()]; exists {
		return fmt.Errorf("instance %s already existing", proc.ID())
	}

	a.instances[proc.ID()] = proc

	a.log.Info("instance register", "id", proc.ID(), "name", proc.Name(), "type", proc.InstanceType())
	return nil
}

// ControlInstance executes a control action on an instance.
func (a *Agent) ControlInstance(instanceID, action string) error {
	a.mu.RLock()
	proc, exists := a.instances[instanceID]
	a.mu.RUnlock()

	if !exists {
		return fmt.Errorf("instance %s none", instanceID)
	}

	switch action {
	case "start":
		return proc.Start()
	case "stop":
		return proc.Stop()
	case "restart":
		return proc.Restart()
	case "kill":
		return proc.Kill()
	default:
		return fmt.Errorf("unknown command: %s", action)
	}
}

// SendCommand sends a console command to an instance and returns the output.
func (a *Agent) SendCommand(instanceID, command string) (string, error) {
	a.mu.RLock()
	proc, exists := a.instances[instanceID]
	a.mu.RUnlock()

	if !exists {
		return "", fmt.Errorf("instance %s none", instanceID)
	}

	return proc.SendCommand(command)
}

// CreateBackup creates a backup for an instance.
func (a *Agent) CreateBackup(instanceID, workDir, label string) (*backup.Result, error) {
	return a.backupMgr.CreateBackup(instanceID, workDir, label)
}

// GetInstanceState returns the state of an instance.
func (a *Agent) GetInstanceState(instanceID string) (process.State, error) {
	a.mu.RLock()
	proc, exists := a.instances[instanceID]
	a.mu.RUnlock()

	if !exists {
		return "", fmt.Errorf("instance %s none", instanceID)
	}

	return proc.State(), nil
}

// RemoveInstance stops and removes an instance from this agent.
// If removeData is true, also deletes the work directory (host volume data).
func (a *Agent) RemoveInstance(instanceID string, removeData bool) error {
	a.mu.Lock()
	proc, exists := a.instances[instanceID]
	def := a.defs[instanceID]
	if !exists {
		a.mu.Unlock()
		return fmt.Errorf("instance %s none", instanceID)
	}
	a.mu.Unlock()

	// 1. runningif first stop
	if proc.State() == process.StateRunning || proc.State() == process.StateStarting {
		a.log.Info("stop instance during", "id", instanceID)
		if err := proc.Stop(); err != nil {
			a.log.Warn("stop instance failed, force shutdown attempt", "id", instanceID, "error", err)
			proc.Kill()
		}
	}

	// 2. Docker remove container
	containerName := fmt.Sprintf("craftstack-%s", proc.Name())
	if a.dockerMgr.ContainerExists(a.ctx, containerName) {
		a.log.Info("Docker remove container", "container", containerName)
		if err := a.dockerMgr.RemoveContainer(a.ctx, containerName, true); err != nil {
			a.log.Warn("Docker remove container failed", "container", containerName, "error", err)
		}
	}

	// 3. inmemory from remove
	a.mu.Lock()
	delete(a.instances, instanceID)
	delete(a.defs, instanceID)
	a.mu.Unlock()

	// 4. data directory delete (option)
	if removeData && def != nil && def.WorkDir != "" {
		a.log.Info("data directory delete", "path", def.WorkDir)
		if err := os.RemoveAll(def.WorkDir); err != nil {
			a.log.Warn("data directory delete failed", "path", def.WorkDir, "error", err)
		}
	}

	a.log.Info("delete instance complete", "id", instanceID, "remove_data", removeData)
	return nil
}

// CreateNewInstance dynamically creates a new service instance as a Docker container.
// For Minecraft: saves the JAR file to the volume directory, creates EULA, then creates container.
// For other types: creates Docker container with the appropriate image.
func (a *Agent) CreateNewInstance(name string, port int, memMin, memMax, jarName string, jarData []byte, autoStart, autoRestart, acceptEULA, startAfterCreate bool, javaPath string, jvmArgs []string, instanceType string, javaVersion string, customDockerfile string, customCompose string, networkName string, zipName string, zipData []byte) (string, error) {
	// name valid check
	if name == "" {
		return "", fmt.Errorf("instance name required")
	}
	if !instanceNameRe.MatchString(name) {
		return "", fmt.Errorf("instance name English, number, hyphen(-), underscore(_), dot(.)only usable and English or number as must start : %s", name)
	}

	// duplicate name check
	instID := fmt.Sprintf("%s-%s", a.cfg.Agent.ID, name)
	a.mu.RLock()
	if _, exists := a.instances[instID]; exists {
		a.mu.RUnlock()
		return "", fmt.Errorf("instance '%s'() already exists", name)
	}
	a.mu.RUnlock()

	// default apply
	if jarName == "" {
		jarName = "server.jar"
	}
	if memMin == "" {
		memMin = "512M"
	}
	if memMax == "" {
		memMax = "1024M"
	}
	if javaPath == "" {
		javaPath = a.cfg.Java.Path
	}
	if port == 0 {
		port = 25565
	}

	// work_dir settings (Docker volume mount target — host filesystem)
	workDir := filepath.Join("./servers", name)

	// type default
	if instanceType == "" {
		instanceType = "minecraft"
	}

	// instance definition create
	def := &common.InstanceDef{
		Type:             instanceType,
		Name:             name,
		Port:             port,
		WorkDir:          workDir,
		ServerJar:        jarName,
		MemoryMin:        memMin,
		MemoryMax:        memMax,
		JavaPath:         javaPath,
		AutoStart:        autoStart,
		AutoRestart:      autoRestart,
		RestartDelaySec:  10,
		StopCommand:      "stop",
		JVMArgs:          jvmArgs,
		AcceptEULA:       acceptEULA,
		JavaVersion:      javaVersion,
		CustomDockerfile: customDockerfile,
		CustomCompose:    customCompose,
		NetworkName:      networkName,
	}

	// work_dir create (Docker volume mount target)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return "", fmt.Errorf("work directory create failed: %w", err)
	}

	// ZIP upload: existing server folder all ZIP as upload case
	if len(zipData) > 0 {
		a.log.Info("server ZIP compress release start", "zip_name", zipName, "zip_size", len(zipData))
		detectedJar, err := a.extractServerZip(zipData, workDir)
		if err != nil {
			return "", fmt.Errorf("server ZIP compress release failed: %w", err)
		}
		a.log.Info("server ZIP compress release complete", "detected_jar", detectedJar)

		// ZIP not from JAR file auto detect case, jarName update
		if instanceType == "minecraft" && detectedJar != "" {
			if jarName == "" || jarName == "server.jar" {
				jarName = detectedJar
				def.ServerJar = jarName
			}
		}
	}

	// Minecraft type: JAR file save + EULA + server.properties
	if instanceType == "minecraft" {
		// JAR-only upload (uploaded JAR only without ZIP case)
		if len(jarData) > 0 && len(zipData) == 0 {
			jarPath := filepath.Join(workDir, jarName)
			if err := os.WriteFile(jarPath, jarData, 0644); err != nil {
				return "", fmt.Errorf("JAR file save failed: %w", err)
			}
			a.log.Info("JAR file save complete", "path", jarPath, "size", len(jarData))
		}

		if acceptEULA {
			eulaPath := filepath.Join(workDir, "eula.txt")
			content := "# CraftStack auto create\n# https://aka.ms/MinecraftEULA\neula=true\n"
			if err := os.WriteFile(eulaPath, []byte(content), 0644); err != nil {
				a.log.Warn("EULA file write failed", "error", err)
			}
		}

		// server.properties if absent min default create file (port apply)
		// server first execute when NoSuchFileException error prevent
		propsPath := filepath.Join(workDir, "server.properties")
		if _, err := os.Stat(propsPath); os.IsNotExist(err) {
			props := fmt.Sprintf("# CraftStack auto create — additional settings auto-added after server start\nserver-port=%d\n", port)
			if err := os.WriteFile(propsPath, []byte(props), 0644); err != nil {
				a.log.Warn("server.properties create failed", "error", err)
			} else {
				a.log.Info("server.properties default create complete", "port", port)
			}
		}
	}

	// Docker create container
	proc, err := a.ensureDockerContainer(instID, def)
	if err != nil {
		return "", fmt.Errorf("Docker create container failed: %w", err)
	}

	if err := a.AddInstance(proc); err != nil {
		return "", fmt.Errorf("instance register failed: %w", err)
	}

	a.mu.Lock()
	a.defs[instID] = def
	a.mu.Unlock()

	a.log.Info("new create instance complete", "id", instID, "name", name, "port", port, "type", instanceType)

	// create after immediately start
	if startAfterCreate {
		a.log.Info("instance auto-start", "name", name)
		if err := a.ControlInstance(instID, "start"); err != nil {
			a.log.Error("instance auto-start failed", "name", name, "error", err)
		}
	}

	return instID, nil
}

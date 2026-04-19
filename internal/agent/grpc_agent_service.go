package agent

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/grpc"

	pb "craftstack/gen/proto/craftstack"
	"craftstack/internal/agent/docker"
	"craftstack/internal/agent/process"
	"craftstack/internal/agent/wireguard"
)

// agentControlServiceImpl implements pb.AgentServiceServer on the Agent side.
// Only ControlInstance and ListInstances are meaningful here.
type agentControlServiceImpl struct {
	pb.UnimplementedAgentServiceServer
	agent *Agent
	log   *slog.Logger
}

// ControlInstance handles start/stop/restart/kill commands from the master.
func (s *agentControlServiceImpl) ControlInstance(ctx context.Context, req *pb.ControlInstanceRequest) (*pb.ControlInstanceResponse, error) {
	s.log.Info("instance control command received",
		"instance_id", req.InstanceId,
		"action", req.Action,
	)

	var action string
	switch req.Action {
	case pb.InstanceAction_INSTANCE_ACTION_START:
		action = "start"
	case pb.InstanceAction_INSTANCE_ACTION_STOP:
		action = "stop"
	case pb.InstanceAction_INSTANCE_ACTION_RESTART:
		action = "restart"
	case pb.InstanceAction_INSTANCE_ACTION_KILL:
		action = "kill"
	default:
		return &pb.ControlInstanceResponse{
			Success: false,
			Message: fmt.Sprintf("unknown action: %v", req.Action),
		}, nil
	}

	if err := s.agent.ControlInstance(req.InstanceId, action); err != nil {
		return &pb.ControlInstanceResponse{
			Success: false,
			Message: fmt.Sprintf("command execution failed: %v", err),
		}, nil
	}

	// current state query
	state, _ := s.agent.GetInstanceState(req.InstanceId)

	return &pb.ControlInstanceResponse{
		Success:  true,
		Message:  "execute command complete",
		NewState: processStateToProto(state),
	}, nil
}

// ListInstances returns all instances managed by this agent.
func (s *agentControlServiceImpl) ListInstances(ctx context.Context, req *pb.ListInstancesRequest) (*pb.ListInstancesResponse, error) {
	s.agent.mu.RLock()
	defer s.agent.mu.RUnlock()

	var instances []*pb.InstanceStatus
	for id, proc := range s.agent.instances {
		instances = append(instances, &pb.InstanceStatus{
			InstanceId:    id,
			Name:          proc.Name(),
			State:         processStateToProto(proc.State()),
			UptimeSeconds: int64(proc.Uptime().Seconds()),
		})
	}

	return &pb.ListInstancesResponse{
		Instances: instances,
	}, nil
}

// BackupInstance triggers a backup for the specified instance.
func (s *agentControlServiceImpl) BackupInstance(ctx context.Context, req *pb.BackupInstanceRequest) (*pb.BackupInstanceResponse, error) {
	s.log.Info("backup request receive", "instance_id", req.InstanceId, "label", req.Label)

	// instance existing check and work_dir query
	s.agent.mu.RLock()
	_, exists := s.agent.instances[req.InstanceId]
	def := s.agent.defs[req.InstanceId]
	s.agent.mu.RUnlock()

	if !exists || def == nil {
		return &pb.BackupInstanceResponse{
			Success: false,
			Message: fmt.Sprintf("instance '%s'() not found", req.InstanceId),
		}, nil
	}

	label := req.Label
	if label == "" {
		label = "manual"
	}

	result, err := s.agent.CreateBackup(req.InstanceId, def.WorkDir, label)
	if err != nil {
		return &pb.BackupInstanceResponse{
			Success: false,
			Message: fmt.Sprintf("backup failed: %v", err),
		}, nil
	}

	// apply retention policy (delete old backups)
	deleted, _ := s.agent.backupMgr.EnforceRetention(req.InstanceId)
	if len(deleted) > 0 {
		s.log.Info("delete old backups", "instance_id", req.InstanceId, "deleted_count", len(deleted))
	}

	return &pb.BackupInstanceResponse{
		Success:  true,
		Message:  "backup complete",
		FilePath: result.FilePath,
		FileSize: result.FileSize,
		Checksum: result.Checksum,
	}, nil
}

// CreateInstance creates a new instance on this agent as a Docker container.
func (s *agentControlServiceImpl) CreateInstance(ctx context.Context, req *pb.CreateInstanceRequest) (*pb.CreateInstanceResponse, error) {
	s.log.Info("create instance request receive",
		"name", req.Name,
		"port", req.Port,
		"type", req.InstanceType,
		"jar", req.ServerJarName,
		"jar_size", len(req.ServerJarData),
	)

	if req.Name == "" {
		return &pb.CreateInstanceResponse{
			Success: false,
			Message: "instance name required",
		}, nil
	}

	instType := req.InstanceType
	if instType == "" {
		instType = "minecraft"
	}

	// Minecraft type: JAR/ZIP data no OK (UploadServerData streaming as during send)
	// JAR nameonly if present proceed available

	networkName := req.NetworkName
	if networkName == "" {
		networkName = "craftstack-default"
	}

	instID, err := s.agent.CreateNewInstance(
		req.Name,
		int(req.Port),
		req.MemoryMin,
		req.MemoryMax,
		req.ServerJarName,
		req.ServerJarData,
		req.AutoStart,
		req.AutoRestart,
		req.AcceptEula,
		req.StartAfterCreate,
		req.JavaPath,
		req.JvmArgs,
		instType,
		req.JavaVersion,
		req.CustomDockerfile,
		req.CustomCompose,
		networkName,
		req.ServerZipName,
		req.ServerZipData,
	)
	if err != nil {
		return &pb.CreateInstanceResponse{
			Success: false,
			Message: fmt.Sprintf("create instance failed: %v", err),
		}, nil
	}

	return &pb.CreateInstanceResponse{
		Success:    true,
		Message:    "the instance created (Docker container)",
		InstanceId: instID,
	}, nil
}

// CheckDocker checks if Docker is installed and running on this agent.
func (s *agentControlServiceImpl) CheckDocker(ctx context.Context, req *pb.CheckDockerRequest) (*pb.CheckDockerResponse, error) {
	mgr := s.agent.dockerMgr

	installed := mgr.IsInstalled()
	running := mgr.IsRunning()

	var version string
	var message string

	if !installed {
		message = "Docker installis not set"
	} else if !running {
		message = "Docker install but daemon execute without "
	} else {
		ver, err := mgr.Version(ctx)
		if err == nil {
			version = ver
		}
		message = "Docker normal running"
	}

	return &pb.CheckDockerResponse{
		Installed: installed,
		Running:   running,
		Version:   version,
		Message:   message,
	}, nil
}

// InstallDocker installs Docker on this agent if not already installed.
func (s *agentControlServiceImpl) InstallDocker(ctx context.Context, req *pb.InstallDockerRequest) (*pb.InstallDockerResponse, error) {
	s.log.Info("Docker install request receive")

	result, err := docker.CheckAndInstall(ctx, s.log)
	if err != nil {
		return &pb.InstallDockerResponse{
			Success: false,
			Message: fmt.Sprintf("Docker install failed: %v", err),
		}, nil
	}

	return &pb.InstallDockerResponse{
		Success: result.Installed,
		Version: result.Version,
		Message: result.Message,
	}, nil
}

// RestoreBackup restores an instance from a backup file.
func (s *agentControlServiceImpl) RestoreBackup(ctx context.Context, req *pb.RestoreBackupRequest) (*pb.RestoreBackupResponse, error) {
	s.log.Info("restore backup request receive", "instance_id", req.InstanceId, "backup_path", req.BackupPath)

	if req.InstanceId == "" || req.BackupPath == "" {
		return &pb.RestoreBackupResponse{
			Success: false,
			Message: "instance ID backup path required",
		}, nil
	}

	// instance existing check
	s.agent.mu.RLock()
	proc, exists := s.agent.instances[req.InstanceId]
	def := s.agent.defs[req.InstanceId]
	s.agent.mu.RUnlock()

	if !exists || def == nil {
		return &pb.RestoreBackupResponse{
			Success: false,
			Message: fmt.Sprintf("instance '%s'() not found", req.InstanceId),
		}, nil
	}

	// restore before stop instance (request when)
	if req.StopBefore && proc.State() == process.StateRunning {
		s.log.Info("restore before stop instance", "instance_id", req.InstanceId)
		if err := proc.Stop(); err != nil {
			s.log.Warn("stop instance failed", "error", err)
		}
	}

	// restore backup
	if err := s.agent.backupMgr.RestoreBackup(req.BackupPath, def.WorkDir); err != nil {
		return &pb.RestoreBackupResponse{
			Success: false,
			Message: fmt.Sprintf("restore backup failed: %v", err),
		}, nil
	}

	return &pb.RestoreBackupResponse{
		Success: true,
		Message: "restore backup complete",
	}, nil
}

// DeleteInstance removes an instance from this agent (stops container, removes it, optionally removes data).
func (s *agentControlServiceImpl) DeleteInstance(ctx context.Context, req *pb.DeleteInstanceRequest) (*pb.DeleteInstanceResponse, error) {
	s.log.Info("delete instance request receive", "instance_id", req.InstanceId, "remove_data", req.RemoveData)

	if req.InstanceId == "" {
		return &pb.DeleteInstanceResponse{
			Success: false,
			Message: "instance ID required",
		}, nil
	}

	if err := s.agent.RemoveInstance(req.InstanceId, req.RemoveData); err != nil {
		return &pb.DeleteInstanceResponse{
			Success: false,
			Message: fmt.Sprintf("delete instance failed: %v", err),
		}, nil
	}

	return &pb.DeleteInstanceResponse{
		Success: true,
		Message: "the instance deleted",
	}, nil
}

// --- Docker Network RPCs ---

// CreateNetwork creates a Docker network on this agent.
func (s *agentControlServiceImpl) CreateNetwork(ctx context.Context, req *pb.CreateNetworkRequest) (*pb.CreateNetworkResponse, error) {
	s.log.Info("create network request receive", "name", req.Name, "driver", req.Driver)

	if req.Name == "" {
		return &pb.CreateNetworkResponse{
			Success: false,
			Message: "network name required",
		}, nil
	}

	networkID, err := s.agent.dockerMgr.NetworkCreate(ctx, req.Name, req.Driver, req.Subnet, req.Gateway)
	if err != nil {
		return &pb.CreateNetworkResponse{
			Success: false,
			Message: fmt.Sprintf("create network failed: %v", err),
		}, nil
	}

	return &pb.CreateNetworkResponse{
		Success:   true,
		Message:   "create network complete",
		NetworkId: networkID,
	}, nil
}

// DeleteNetwork removes a Docker network from this agent.
func (s *agentControlServiceImpl) DeleteNetwork(ctx context.Context, req *pb.DeleteNetworkRequest) (*pb.DeleteNetworkResponse, error) {
	s.log.Info("delete network request receive", "name", req.Name)

	if req.Name == "" {
		return &pb.DeleteNetworkResponse{
			Success: false,
			Message: "network name required",
		}, nil
	}

	if err := s.agent.dockerMgr.NetworkRemove(ctx, req.Name); err != nil {
		return &pb.DeleteNetworkResponse{
			Success: false,
			Message: fmt.Sprintf("delete network failed: %v", err),
		}, nil
	}

	return &pb.DeleteNetworkResponse{
		Success: true,
		Message: "delete network complete",
	}, nil
}

// ListNetworks lists all Docker networks on this agent.
func (s *agentControlServiceImpl) ListNetworks(ctx context.Context, req *pb.ListNetworksRequest) (*pb.ListNetworksResponse, error) {
	networks, err := s.agent.dockerMgr.NetworkList(ctx)
	if err != nil {
		return nil, fmt.Errorf("network list query failed: %w", err)
	}

	var pbNetworks []*pb.DockerNetworkInfo
	for _, n := range networks {
		info := &pb.DockerNetworkInfo{
			Id:             n.ID,
			Name:           n.Name,
			Driver:         n.Driver,
			Scope:          n.Scope,
			ContainerCount: int32(len(n.Containers)),
		}
		if len(n.IPAM.Config) > 0 {
			info.Subnet = n.IPAM.Config[0].Subnet
			info.Gateway = n.IPAM.Config[0].Gateway
		}
		pbNetworks = append(pbNetworks, info)
	}

	return &pb.ListNetworksResponse{
		Networks: pbNetworks,
	}, nil
}

// ConnectNetwork connects a container to a Docker network.
func (s *agentControlServiceImpl) ConnectNetwork(ctx context.Context, req *pb.ConnectNetworkRequest) (*pb.ConnectNetworkResponse, error) {
	s.log.Info("connect network request", "network", req.NetworkName, "container", req.ContainerName)

	if req.NetworkName == "" || req.ContainerName == "" {
		return &pb.ConnectNetworkResponse{
			Success: false,
			Message: "network name container name required",
		}, nil
	}

	if err := s.agent.dockerMgr.NetworkConnect(ctx, req.NetworkName, req.ContainerName, req.Alias, req.IpAddress); err != nil {
		return &pb.ConnectNetworkResponse{
			Success: false,
			Message: fmt.Sprintf("connect network failed: %v", err),
		}, nil
	}

	return &pb.ConnectNetworkResponse{
		Success: true,
		Message: "connect network complete",
	}, nil
}

// DisconnectNetwork disconnects a from containers a Docker network.
func (s *agentControlServiceImpl) DisconnectNetwork(ctx context.Context, req *pb.DisconnectNetworkRequest) (*pb.DisconnectNetworkResponse, error) {
	s.log.Info("connect network release request", "network", req.NetworkName, "container", req.ContainerName)

	if req.NetworkName == "" || req.ContainerName == "" {
		return &pb.DisconnectNetworkResponse{
			Success: false,
			Message: "network name container name required",
		}, nil
	}

	if err := s.agent.dockerMgr.NetworkDisconnect(ctx, req.NetworkName, req.ContainerName); err != nil {
		return &pb.DisconnectNetworkResponse{
			Success: false,
			Message: fmt.Sprintf("connect network release failed: %v", err),
		}, nil
	}

	return &pb.DisconnectNetworkResponse{
		Success: true,
		Message: "connect network release complete",
	}, nil
}

// --- WireGuard Mesh RPCs ---

// ConfigureWireGuard applies WireGuard tunnel configuration on this agent.
func (s *agentControlServiceImpl) ConfigureWireGuard(ctx context.Context, req *pb.ConfigureWireGuardRequest) (*pb.ConfigureWireGuardResponse, error) {
	s.log.Info("WireGuard settings receive",
		"address", req.Address,
		"listen_port", req.ListenPort,
		"peers", len(req.Peers),
		"dns_listen_ip", req.DnsListenIp,
	)

	if s.agent.wgMgr == nil {
		return &pb.ConfigureWireGuardResponse{
			Success: false,
			Message: "WireGuard admin not initialized",
		}, nil
	}

	// Build WG config from proto request
	var peers []wgPeerConfig
	for _, p := range req.Peers {
		peers = append(peers, wgPeerConfig{
			PublicKey:  p.PublicKey,
			Endpoint:   p.Endpoint,
			AllowedIPs: p.AllowedIps,
			Keepalive:  int(p.Keepalive),
		})
	}

	if err := s.agent.applyWireGuardConfig(ctx, req.PrivateKey, req.Address, int(req.ListenPort), peers, req.DnsListenIp); err != nil {
		return &pb.ConfigureWireGuardResponse{
			Success: false,
			Message: fmt.Sprintf("WireGuard settings apply failed: %v", err),
		}, nil
	}

	return &pb.ConfigureWireGuardResponse{
		Success: true,
		Message: "WireGuard settings apply complete",
	}, nil
}

// WireGuardStatus returns the WireGuard installation and tunnel status.
// If WireGuard is not installed, it automatically attempts installation.
func (s *agentControlServiceImpl) WireGuardStatus(ctx context.Context, req *pb.WireGuardStatusRequest) (*pb.WireGuardStatusResponse, error) {
	resp := &pb.WireGuardStatusResponse{}

	if s.agent.wgMgr == nil {
		return resp, nil
	}

	// notinstall when auto install attempt
	if !s.agent.wgMgr.IsInstalled() {
		s.log.Info("WireGuard notinstall detect, auto install attempt")
		if err := wireguard.EnsureWireGuard(ctx, s.log); err != nil {
			s.log.Warn("WireGuard auto install failed", "error", err)
			return resp, nil
		}
		// install after Manager recreate new path detect
		s.agent.wgMgr = wireguard.NewManager(s.log)
		s.log.Info("WireGuard auto install complete")
	}

	resp.Installed = s.agent.wgMgr.IsInstalled()
	resp.Active = s.agent.wgMgr.IsActive()
	resp.PublicKey = s.agent.wgMgr.PublicKey()
	resp.Address = s.agent.wgMgr.Address()

	// If installed but no key yet, generate one
	if resp.Installed && resp.PublicKey == "" {
		_, pubKey, err := s.agent.wgMgr.GenerateKeyPair(ctx)
		if err == nil {
			resp.PublicKey = pubKey
		} else {
			s.log.Warn("WG key creation failed", "error", err)
		}
	}

	// Get peer status
	if resp.Active {
		peers, err := s.agent.wgMgr.Status(ctx)
		if err == nil {
			for _, p := range peers {
				resp.Peers = append(resp.Peers, &pb.WireGuardPeerStatus{
					PublicKey:         p.PublicKey,
					Endpoint:          p.Endpoint,
					LastHandshakeUnix: p.LastHandshake.Unix(),
					RxBytes:           p.RxBytes,
					TxBytes:           p.TxBytes,
					Connected:         p.Connected,
				})
			}
		}
	}

	return resp, nil
}

// UpdateDNSRecords pushes DNS records to the agent's embedded DNS server.
func (s *agentControlServiceImpl) UpdateDNSRecords(ctx context.Context, req *pb.UpdateDNSRecordsRequest) (*pb.UpdateDNSRecordsResponse, error) {
	if s.agent.dnsServer == nil {
		return &pb.UpdateDNSRecordsResponse{
			Success: false,
			Message: "DNS server not initialized",
		}, nil
	}

	s.agent.updateDNSRecords(req.Records)

	return &pb.UpdateDNSRecordsResponse{
		Success:     true,
		Message:     "DNS record update complete",
		RecordCount: int32(s.agent.dnsServer.RecordCount()),
	}, nil
}

// UploadServerData handles client-streaming upload of large JAR/ZIP files.
// The first message contains metadata (instance_name, file_type, file_name, total_size).
// Subsequent messages contain chunk_data (~4MB each).
// On completion, the file is placed in the instance's work_dir.
func (s *agentControlServiceImpl) UploadServerData(stream grpc.ClientStreamingServer[pb.UploadServerDataRequest, pb.UploadServerDataResponse]) error {
	var (
		instanceName string
		fileType     string // "jar" or "zip"
		fileName     string
		totalSize    int64
		tmpFile      *os.File
		received     int64
		firstChunk   = true
	)

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			s.log.Error("streaming receive error", "error", err)
			return err
		}

		// first th chunk from metadata extract
		if firstChunk {
			instanceName = req.InstanceName
			fileType = req.FileType
			fileName = req.FileName
			totalSize = req.TotalSize
			firstChunk = false

			s.log.Info("file upload streaming start",
				"instance", instanceName,
				"type", fileType,
				"file", fileName,
				"total_size", totalSize,
			)

			if instanceName == "" || fileName == "" {
				return stream.SendAndClose(&pb.UploadServerDataResponse{
					Success: false,
					Message: "instance name file name required",
				})
			}

			// temporary create file
			tmpFile, err = os.CreateTemp("", "craftstack-upload-*")
			if err != nil {
				return stream.SendAndClose(&pb.UploadServerDataResponse{
					Success: false,
					Message: fmt.Sprintf("temporary create file failed: %v", err),
				})
			}
			defer os.Remove(tmpFile.Name())
			defer tmpFile.Close()
		}

		// chunk data write
		if len(req.ChunkData) > 0 && tmpFile != nil {
			n, err := tmpFile.Write(req.ChunkData)
			if err != nil {
				return stream.SendAndClose(&pb.UploadServerDataResponse{
					Success: false,
					Message: fmt.Sprintf("file write failed: %v", err),
				})
			}
			received += int64(n)
		}
	}

	if tmpFile == nil {
		return stream.SendAndClose(&pb.UploadServerDataResponse{
			Success: false,
			Message: "data receivecannotdone",
		})
	}

	// save to temp file first
	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		return stream.SendAndClose(&pb.UploadServerDataResponse{
			Success: false,
			Message: fmt.Sprintf("file save failed: %v", err),
		})
	}

	s.log.Info("file upload receive complete",
		"instance", instanceName,
		"received", received,
		"total", totalSize,
	)

	// instance work_dir decide
	workDir := filepath.Join("./servers", instanceName)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return stream.SendAndClose(&pb.UploadServerDataResponse{
			Success: false,
			Message: fmt.Sprintf("work directory create failed: %v", err),
		})
	}

	var detectedJar string

	switch strings.ToLower(fileType) {
	case "jar":
		// JAR: copy directly to work_dir
		destPath := filepath.Join(workDir, fileName)
		destFile, err := os.Create(destPath)
		if err != nil {
			return stream.SendAndClose(&pb.UploadServerDataResponse{
				Success: false,
				Message: fmt.Sprintf("JAR create file failed: %v", err),
			})
		}
		if _, err := io.Copy(destFile, tmpFile); err != nil {
			destFile.Close()
			return stream.SendAndClose(&pb.UploadServerDataResponse{
				Success: false,
				Message: fmt.Sprintf("JAR file copy failed: %v", err),
			})
		}
		destFile.Close()
		s.log.Info("JAR file save complete", "path", destPath, "size", received)

	case "zip":
		// ZIP: temporary file from read extractServerZip call
		// extractServerZip []byte receive, so read in
		zipData, err := io.ReadAll(tmpFile)
		if err != nil {
			return stream.SendAndClose(&pb.UploadServerDataResponse{
				Success: false,
				Message: fmt.Sprintf("ZIP data read failed: %v", err),
			})
		}
		detected, err := s.agent.extractServerZip(zipData, workDir)
		if err != nil {
			return stream.SendAndClose(&pb.UploadServerDataResponse{
				Success: false,
				Message: fmt.Sprintf("ZIP compress release failed: %v", err),
			})
		}
		detectedJar = detected
		s.log.Info("ZIP compress release complete", "detected_jar", detectedJar)

	default:
		return stream.SendAndClose(&pb.UploadServerDataResponse{
			Success: false,
			Message: fmt.Sprintf("support not file type: %s", fileType),
		})
	}

	// instance definition update (ZIP from JAR detect when)
	if detectedJar != "" {
		s.agent.mu.Lock()
		instID := fmt.Sprintf("%s-%s", s.agent.cfg.Agent.ID, instanceName)
		if def, ok := s.agent.defs[instID]; ok {
			if def.ServerJar == "" || def.ServerJar == "server.jar" {
				def.ServerJar = detectedJar
			}
		}
		s.agent.mu.Unlock()
	}

	return stream.SendAndClose(&pb.UploadServerDataResponse{
		Success:     true,
		Message:     "file upload complete",
		DetectedJar: detectedJar,
	})
}

// wgPeerConfig is an internal peer config struct for the agent.
type wgPeerConfig struct {
	PublicKey  string
	Endpoint   string
	AllowedIPs []string
	Keepalive  int
}

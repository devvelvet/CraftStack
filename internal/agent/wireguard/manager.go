package wireguard

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Manager handles WireGuard interface setup and peer management.
type Manager struct {
	log        *slog.Logger
	wgPath     string // "wg" or full path
	ifName     string // interface name (wg-craftstack)
	configDir  string // config file directory
	mu         sync.Mutex
	active     bool
	address    string // current WG address (e.g., "10.10.0.1/16")
	listenPort int
	publicKey  string
}

// PeerConfig holds WireGuard peer configuration.
type PeerConfig struct {
	PublicKey  string
	Endpoint   string   // "IP:port"
	AllowedIPs []string // CIDR list
	Keepalive  int      // seconds, 0 = disabled
}

// Config holds the full WireGuard interface configuration.
type Config struct {
	PrivateKey  string
	Address     string // "10.10.0.1/16"
	ListenPort  int
	Peers       []PeerConfig
	DNSListenIP string // IP for DNS server binding
}

// PeerStatus holds runtime status of a WireGuard peer.
type PeerStatus struct {
	PublicKey     string
	Endpoint      string
	LastHandshake time.Time
	RxBytes       int64
	TxBytes       int64
	Connected     bool
}

// NewManager creates a new WireGuard manager.
func NewManager(log *slog.Logger) *Manager {
	ifName := "wg-craftstack"
	if runtime.GOOS == "windows" {
		ifName = "wg-craftstack" // Windows uses tunnel name
	}

	configDir := "/etc/wireguard"
	if runtime.GOOS == "windows" {
		configDir = filepath.Join(os.Getenv("ProgramData"), "WireGuard")
	}

	return &Manager{
		log:       log,
		wgPath:    findWGPath(),
		ifName:    ifName,
		configDir: configDir,
	}
}

// IsInstalled checks if WireGuard tools are available.
func (m *Manager) IsInstalled() bool {
	return m.wgPath != ""
}

// IsActive returns whether the WireGuard tunnel is currently active.
func (m *Manager) IsActive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active
}

// PublicKey returns the current public key.
func (m *Manager) PublicKey() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.publicKey
}

// Address returns the current WG address.
func (m *Manager) Address() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.address
}

// GenerateKeyPair generates a WireGuard key pair.
func (m *Manager) GenerateKeyPair(ctx context.Context) (privateKey, publicKey string, err error) {
	if !m.IsInstalled() {
		return "", "", fmt.Errorf("WireGuard installis not set")
	}

	// Generate private key
	privCmd := exec.CommandContext(ctx, m.wgPath, "genkey")
	privOut, err := privCmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("wg genkey failed: %w", err)
	}
	privateKey = strings.TrimSpace(string(privOut))

	// Derive public key
	pubCmd := exec.CommandContext(ctx, m.wgPath, "pubkey")
	pubCmd.Stdin = strings.NewReader(privateKey)
	pubOut, err := pubCmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("wg pubkey failed: %w", err)
	}
	publicKey = strings.TrimSpace(string(pubOut))

	return privateKey, publicKey, nil
}

// ApplyConfig writes the WireGuard configuration and activates the interface.
func (m *Manager) ApplyConfig(ctx context.Context, cfg *Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.IsInstalled() {
		return fmt.Errorf("WireGuard installis not set")
	}

	// Ensure config directory exists
	os.MkdirAll(m.configDir, 0700)

	if runtime.GOOS == "windows" {
		return m.applyConfigWindows(ctx, cfg)
	}
	return m.applyConfigLinux(ctx, cfg)
}

// applyConfigLinux handles Linux WireGuard setup using wg and ip commands.
func (m *Manager) applyConfigLinux(ctx context.Context, cfg *Config) error {
	confPath := filepath.Join(m.configDir, m.ifName+".conf")

	// Write wg-quick compatible config file
	content := m.buildConfigFile(cfg)
	if err := os.WriteFile(confPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("config file save failed: %w", err)
	}

	// Check if interface already exists
	checkCmd := exec.CommandContext(ctx, "ip", "link", "show", m.ifName)
	if err := checkCmd.Run(); err == nil {
		// Interface exists — reload config
		m.log.Info("existing WireGuard interface settings refresh", "interface", m.ifName)
		syncCmd := exec.CommandContext(ctx, m.wgPath, "syncconf", m.ifName, confPath)
		if out, err := syncCmd.CombinedOutput(); err != nil {
			// syncconf doesn't support wg-quick format, use strip
			stripCmd := exec.CommandContext(ctx, "bash", "-c",
				fmt.Sprintf("wg-quick strip %s | wg syncconf %s /dev/stdin", m.ifName, m.ifName))
			if out2, err2 := stripCmd.CombinedOutput(); err2 != nil {
				m.log.Warn("wg syncconf failed, interface restart", "error", string(out), "error2", string(out2))
				// Fallback: restart interface
				exec.CommandContext(ctx, "wg-quick", "down", m.ifName).Run()
				if out3, err3 := exec.CommandContext(ctx, "wg-quick", "up", m.ifName).CombinedOutput(); err3 != nil {
					return fmt.Errorf("wg-quick up failed: %s: %w", string(out3), err3)
				}
			}
		}
	} else {
		// Interface doesn't exist — bring up
		m.log.Info("WireGuard interface start", "interface", m.ifName)
		upCmd := exec.CommandContext(ctx, "wg-quick", "up", m.ifName)
		if out, err := upCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("wg-quick up failed: %s: %w", strings.TrimSpace(string(out)), err)
		}
	}

	m.active = true
	m.address = cfg.Address
	m.listenPort = cfg.ListenPort
	m.publicKey = m.derivePublicKey(ctx, cfg.PrivateKey)
	return nil
}

// applyConfigWindows handles Windows WireGuard setup.
func (m *Manager) applyConfigWindows(ctx context.Context, cfg *Config) error {
	confPath := filepath.Join(m.configDir, m.ifName+".conf")

	content := m.buildConfigFile(cfg)
	if err := os.WriteFile(confPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("config file save failed: %w", err)
	}

	// On Windows, we use the wireguard.exe service mechanism
	// First try to remove existing tunnel, then install
	wireguardExe := findWireGuardExe()
	if wireguardExe == "" {
		return fmt.Errorf("wireguard.exe not found")
	}

	// Remove existing tunnel service (ignore errors)
	exec.CommandContext(ctx, wireguardExe, "/uninstalltunnelservice", m.ifName).Run()
	time.Sleep(2 * time.Second)

	// Install new tunnel service
	installCmd := exec.CommandContext(ctx, wireguardExe, "/installtunnelservice", confPath)
	if out, err := installCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wireguard tunnel service install failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	// Wait for tunnel to come up
	time.Sleep(3 * time.Second)

	m.active = true
	m.address = cfg.Address
	m.listenPort = cfg.ListenPort
	m.publicKey = m.derivePublicKey(ctx, cfg.PrivateKey)
	return nil
}

// buildConfigFile generates a wg-quick compatible INI config.
func (m *Manager) buildConfigFile(cfg *Config) string {
	var sb strings.Builder

	// Extract IP from CIDR for PostUp routing
	wgIP, _, _ := net.ParseCIDR(cfg.Address)

	sb.WriteString("[Interface]\n")
	sb.WriteString(fmt.Sprintf("PrivateKey = %s\n", cfg.PrivateKey))
	sb.WriteString(fmt.Sprintf("Address = %s\n", cfg.Address))
	sb.WriteString(fmt.Sprintf("ListenPort = %d\n", cfg.ListenPort))

	if runtime.GOOS == "linux" {
		// Enable IP forwarding for cross-network routing
		sb.WriteString("PostUp = sysctl -w net.ipv4.ip_forward=1\n")
		// Add routes for Docker subnets of peers via wg interface
		// (handled by AllowedIPs routing automatically with wg-quick)
	}

	_ = wgIP // used in comments above

	for _, peer := range cfg.Peers {
		sb.WriteString("\n[Peer]\n")
		sb.WriteString(fmt.Sprintf("PublicKey = %s\n", peer.PublicKey))
		if peer.Endpoint != "" {
			sb.WriteString(fmt.Sprintf("Endpoint = %s\n", peer.Endpoint))
		}
		if len(peer.AllowedIPs) > 0 {
			sb.WriteString(fmt.Sprintf("AllowedIPs = %s\n", strings.Join(peer.AllowedIPs, ", ")))
		}
		keepalive := peer.Keepalive
		if keepalive <= 0 {
			keepalive = 25 // Default for NAT traversal
		}
		sb.WriteString(fmt.Sprintf("PersistentKeepalive = %d\n", keepalive))
	}

	return sb.String()
}

// Status returns the current WireGuard interface status and peer information.
func (m *Manager) Status(ctx context.Context) ([]PeerStatus, error) {
	if !m.IsInstalled() {
		return nil, fmt.Errorf("WireGuard installis not set")
	}

	// Use wg show with JSON-like output
	cmd := exec.CommandContext(ctx, m.wgPath, "show", m.ifName, "dump")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("wg show failed: %w", err)
	}

	return parseDump(string(out)), nil
}

// InterfaceDown tears down the WireGuard interface.
func (m *Manager) InterfaceDown(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if runtime.GOOS == "windows" {
		wireguardExe := findWireGuardExe()
		if wireguardExe != "" {
			exec.CommandContext(ctx, wireguardExe, "/uninstalltunnelservice", m.ifName).Run()
		}
	} else {
		exec.CommandContext(ctx, "wg-quick", "down", m.ifName).Run()
	}

	m.active = false
	return nil
}

// derivePublicKey derives public key from private key.
func (m *Manager) derivePublicKey(ctx context.Context, privateKey string) string {
	cmd := exec.CommandContext(ctx, m.wgPath, "pubkey")
	cmd.Stdin = strings.NewReader(privateKey)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// parseDump parses "wg show <iface> dump" output into PeerStatus slices.
// Format: each line (tab-separated): public_key, preshared_key, endpoint, allowed_ips, latest_handshake, rx, tx, keepalive
// First line is the interface itself.
func parseDump(output string) []PeerStatus {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		return nil
	}

	var peers []PeerStatus
	for _, line := range lines[1:] { // skip interface line
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}

		var handshakeUnix int64
		var handshakeTime time.Time
		var rxBytes, txBytes int64
		fmt.Sscanf(fields[4], "%d", &handshakeUnix)
		if handshakeUnix > 0 {
			handshakeTime = time.Unix(handshakeUnix, 0)
		}
		fmt.Sscanf(fields[5], "%d", &rxBytes)
		fmt.Sscanf(fields[6], "%d", &txBytes)

		connected := handshakeUnix > 0 && time.Since(handshakeTime) < 3*time.Minute

		peers = append(peers, PeerStatus{
			PublicKey:     fields[0],
			Endpoint:      fields[2],
			LastHandshake: handshakeTime,
			RxBytes:       rxBytes,
			TxBytes:       txBytes,
			Connected:     connected,
		})
	}
	return peers
}

// findWGPath finds the wg command path.
func findWGPath() string {
	if runtime.GOOS == "windows" {
		// Windows: check common install locations
		paths := []string{
			filepath.Join(os.Getenv("ProgramFiles"), "WireGuard", "wg.exe"),
			filepath.Join(os.Getenv("SystemRoot"), "System32", "wg.exe"),
		}
		for _, p := range paths {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
		// Try PATH
		if p, err := exec.LookPath("wg.exe"); err == nil {
			return p
		}
		return ""
	}

	// Linux/macOS
	if p, err := exec.LookPath("wg"); err == nil {
		return p
	}
	// Common locations
	for _, p := range []string{"/usr/bin/wg", "/usr/local/bin/wg"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// findWireGuardExe finds wireguard.exe on Windows.
func findWireGuardExe() string {
	paths := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "WireGuard", "wireguard.exe"),
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if p, err := exec.LookPath("wireguard.exe"); err == nil {
		return p
	}
	return ""
}

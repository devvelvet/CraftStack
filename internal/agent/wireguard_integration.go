package agent

import (
	"context"
	"fmt"

	pb "craftstack/gen/proto/craftstack"
	agentdns "craftstack/internal/agent/dns"
	"craftstack/internal/agent/wireguard"
)

// applyWireGuardConfig applies WireGuard configuration and starts the DNS server.
func (a *Agent) applyWireGuardConfig(ctx context.Context, privateKey, address string, listenPort int, peers []wgPeerConfig, dnsListenIP string) error {
	if a.wgMgr == nil {
		return fmt.Errorf("WireGuard admin not initialized")
	}

	// Install WireGuard if needed
	if !a.wgMgr.IsInstalled() {
		a.log.Info("WireGuard auto install attempt")
		if err := wireguard.EnsureWireGuard(ctx, a.log); err != nil {
			return fmt.Errorf("WireGuard install failed: %w", err)
		}
		// Re-create manager to pick up new path
		a.wgMgr = wireguard.NewManager(a.log)
	}

	// Build WG config
	var wgPeers []wireguard.PeerConfig
	for _, p := range peers {
		wgPeers = append(wgPeers, wireguard.PeerConfig{
			PublicKey:  p.PublicKey,
			Endpoint:   p.Endpoint,
			AllowedIPs: p.AllowedIPs,
			Keepalive:  p.Keepalive,
		})
	}

	cfg := &wireguard.Config{
		PrivateKey:  privateKey,
		Address:     address,
		ListenPort:  listenPort,
		Peers:       wgPeers,
		DNSListenIP: dnsListenIP,
	}

	if err := a.wgMgr.ApplyConfig(ctx, cfg); err != nil {
		return fmt.Errorf("WireGuard settings apply failed: %w", err)
	}

	// Start DNS server on WireGuard IP (if not already running)
	if dnsListenIP != "" && a.dnsServer == nil {
		a.wgDNSListenIP = dnsListenIP
		a.dnsServer = agentdns.NewServer(a.log, dnsListenIP, 53, "craftstack.internal")
		if err := a.dnsServer.Start(); err != nil {
			a.log.Warn("embedded DNS server start failed (ignore)", "error", err)
			a.dnsServer = nil
		} else {
			a.log.Info("embedded DNS server start complete", "listen_ip", dnsListenIP)
		}
	}

	return nil
}

// updateDNSRecords updates the embedded DNS server's records.
func (a *Agent) updateDNSRecords(records []*pb.DNSRecord) {
	if a.dnsServer == nil {
		return
	}

	var dnsRecords []agentdns.Record
	for _, r := range records {
		dnsRecords = append(dnsRecords, agentdns.Record{
			FQDN:      r.Fqdn,
			IPAddress: r.IpAddress,
		})
	}

	a.dnsServer.UpdateRecords(dnsRecords)
}

// WGDNSListenIP returns the WireGuard IP used for DNS.
func (a *Agent) WGDNSListenIP() string {
	return a.wgDNSListenIP
}

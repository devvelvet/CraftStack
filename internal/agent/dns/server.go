package dns

import (
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"

	mdns "github.com/miekg/dns"
)

// Record holds a DNS A record.
type Record struct {
	FQDN      string // "main-db.craftstack.internal."
	IPAddress string // "172.30.2.10"
}

// Server is a lightweight DNS server for cross-node service discovery.
// It resolves *.craftstack.internal queries to Docker container IPs.
// Other queries are forwarded to system DNS.
//
// Port 53 is required because Docker's --dns flag only accepts an IP address
// and always uses port 53. The server binds to the WireGuard tunnel IP
// (not 0.0.0.0), so it won't conflict with systemd-resolved (127.0.0.53).
// Both Linux and Windows agents already run with elevated privileges
// (required for Docker and WireGuard), so port 53 binding is permitted.
type Server struct {
	log      *slog.Logger
	listenIP string // WireGuard IP to bind to
	port     int    // DNS port (53)
	domain   string // "craftstack.internal."
	server   *mdns.Server

	mu      sync.RWMutex
	records map[string]string // FQDN (with trailing dot) → IP
}

// NewServer creates a new DNS server.
// listenIP should be the WireGuard tunnel IP (e.g., "10.10.0.1").
// domain should be "craftstack.internal" (without trailing dot).
func NewServer(log *slog.Logger, listenIP string, port int, domain string) *Server {
	if port == 0 {
		port = 53
	}
	if domain == "" {
		domain = "craftstack.internal"
	}
	// Ensure domain ends with dot for DNS
	if !strings.HasSuffix(domain, ".") {
		domain = domain + "."
	}

	return &Server{
		log:      log,
		listenIP: listenIP,
		port:     port,
		domain:   domain,
		records:  make(map[string]string),
	}
}

// Start begins listening for DNS queries on the WireGuard IP.
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.listenIP, s.port)

	mux := mdns.NewServeMux()
	mux.HandleFunc(s.domain, s.handleQuery)
	mux.HandleFunc(".", s.handleForward)

	s.server = &mdns.Server{
		Addr:    addr,
		Net:     "udp",
		Handler: mux,
	}

	s.log.Info("embedded DNS server start", "addr", addr, "domain", s.domain)

	go func() {
		if err := s.server.ListenAndServe(); err != nil {
			s.log.Error("DNS server error", "error", err)
		}
	}()

	return nil
}

// Stop shuts down the DNS server.
func (s *Server) Stop() {
	if s.server != nil {
		s.server.Shutdown()
	}
}

// UpdateRecords replaces all DNS records.
func (s *Server) UpdateRecords(records []Record) {
	s.mu.Lock()
	defer s.mu.Unlock()

	newMap := make(map[string]string, len(records))
	for _, r := range records {
		fqdn := r.FQDN
		if !strings.HasSuffix(fqdn, ".") {
			fqdn = fqdn + "."
		}
		fqdn = strings.ToLower(fqdn)
		newMap[fqdn] = r.IPAddress
	}
	s.records = newMap
	s.log.Debug("DNS record update", "count", len(newMap))
}

// RecordCount returns the number of active DNS records.
func (s *Server) RecordCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.records)
}

// handleQuery handles DNS queries for our domain (*.craftstack.internal).
func (s *Server) handleQuery(w mdns.ResponseWriter, r *mdns.Msg) {
	msg := new(mdns.Msg)
	msg.SetReply(r)
	msg.Authoritative = true

	for _, q := range r.Question {
		if q.Qtype != mdns.TypeA && q.Qtype != mdns.TypeAAAA {
			continue
		}

		name := strings.ToLower(q.Name)

		s.mu.RLock()
		ip, found := s.records[name]
		s.mu.RUnlock()

		if found && q.Qtype == mdns.TypeA {
			parsedIP := net.ParseIP(ip)
			if parsedIP != nil && parsedIP.To4() != nil {
				rr := &mdns.A{
					Hdr: mdns.RR_Header{
						Name:   q.Name,
						Rrtype: mdns.TypeA,
						Class:  mdns.ClassINET,
						Ttl:    30, // Short TTL for dynamic environments
					},
					A: parsedIP.To4(),
				}
				msg.Answer = append(msg.Answer, rr)
				s.log.Debug("DNS query response", "query", name, "ip", ip)
			}
		}
	}

	if len(msg.Answer) == 0 {
		msg.Rcode = mdns.RcodeNameError // NXDOMAIN
	}

	w.WriteMsg(msg)
}

// handleForward forwards non-domain queries to system DNS (8.8.8.8 as fallback).
func (s *Server) handleForward(w mdns.ResponseWriter, r *mdns.Msg) {
	// Try system resolvers, fall back to public DNS
	resolvers := getSystemResolvers()
	if len(resolvers) == 0 {
		resolvers = []string{"8.8.8.8:53", "1.1.1.1:53"}
	}

	client := &mdns.Client{Net: "udp"}
	for _, resolver := range resolvers {
		// Skip ourselves
		if strings.HasPrefix(resolver, s.listenIP+":") {
			continue
		}
		resp, _, err := client.Exchange(r, resolver)
		if err == nil {
			w.WriteMsg(resp)
			return
		}
	}

	// All resolvers failed — return SERVFAIL
	msg := new(mdns.Msg)
	msg.SetReply(r)
	msg.Rcode = mdns.RcodeServerFailure
	w.WriteMsg(msg)
}

// getSystemResolvers reads system DNS configuration.
func getSystemResolvers() []string {
	// miekg/dns provides a helper for this
	config, err := mdns.ClientConfigFromFile("/etc/resolv.conf")
	if err != nil {
		return nil
	}
	var resolvers []string
	for _, srv := range config.Servers {
		resolvers = append(resolvers, net.JoinHostPort(srv, config.Port))
	}
	return resolvers
}

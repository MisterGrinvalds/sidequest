package dns

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/miekg/dns"
)

// Server is a DNS server with configurable zone data.
type Server struct {
	port     int
	zones    map[string][]dns.RR
	upstream string
	udp      *dns.Server
	tcp      *dns.Server
}

// New creates a new DNS server.
func New(port int) *Server {
	s := &Server{
		port:     port,
		zones:    make(map[string][]dns.RR),
		upstream: "8.8.8.8:53",
	}

	// Load zones from environment variables.
	s.loadZonesFromEnv()

	// Add default sidequest.local zone.
	s.addDefaultZone()

	return s
}

// Port returns the configured port.
func (s *Server) Port() int { return s.port }

// Start begins serving DNS on UDP and TCP.
func (s *Server) Start() error {
	mux := dns.NewServeMux()
	mux.HandleFunc(".", s.handleDNS)

	addr := fmt.Sprintf(":%d", s.port)

	s.udp = &dns.Server{Addr: addr, Net: "udp", Handler: mux}
	s.tcp = &dns.Server{Addr: addr, Net: "tcp", Handler: mux}

	errCh := make(chan error, 2)
	go func() { errCh <- s.udp.ListenAndServe() }()
	go func() { errCh <- s.tcp.ListenAndServe() }()

	return <-errCh
}

// Shutdown stops the DNS server.
func (s *Server) Shutdown() {
	if s.udp != nil {
		s.udp.Shutdown()
	}
	if s.tcp != nil {
		s.tcp.Shutdown()
	}
}

func (s *Server) handleDNS(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	for _, q := range r.Question {
		name := strings.ToLower(q.Name)
		if records, ok := s.zones[name]; ok {
			for _, rr := range records {
				if rr.Header().Rrtype == q.Qtype || q.Qtype == dns.TypeANY {
					m.Answer = append(m.Answer, rr)
				}
			}
		}
	}

	// If no local answers, forward upstream.
	if len(m.Answer) == 0 && s.upstream != "" {
		resp, err := dns.Exchange(r, s.upstream)
		if err == nil && resp != nil {
			resp.Id = r.Id
			w.WriteMsg(resp)
			return
		}
	}

	w.WriteMsg(m)
}

func (s *Server) loadZonesFromEnv() {
	// Parse env vars like SIDEQUEST_DNS_ZONE_example_A=1.2.3.4
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "SIDEQUEST_DNS_ZONE_") {
			continue
		}
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimPrefix(parts[0], "SIDEQUEST_DNS_ZONE_")
		value := parts[1]

		// Key format: <name>_<type> e.g. example.com_A
		idx := strings.LastIndex(key, "_")
		if idx < 0 {
			continue
		}
		name := strings.ReplaceAll(key[:idx], "_", ".") + "."
		rrType := strings.ToUpper(key[idx+1:])

		var rr dns.RR
		var err error
		switch rrType {
		case "A":
			rr, err = dns.NewRR(fmt.Sprintf("%s 300 IN A %s", name, value))
		case "AAAA":
			rr, err = dns.NewRR(fmt.Sprintf("%s 300 IN AAAA %s", name, value))
		case "CNAME":
			if !strings.HasSuffix(value, ".") {
				value += "."
			}
			rr, err = dns.NewRR(fmt.Sprintf("%s 300 IN CNAME %s", name, value))
		case "MX":
			rr, err = dns.NewRR(fmt.Sprintf("%s 300 IN MX 10 %s", name, value))
		case "TXT":
			rr, err = dns.NewRR(fmt.Sprintf(`%s 300 IN TXT "%s"`, name, value))
		}

		if err == nil && rr != nil {
			s.zones[strings.ToLower(name)] = append(s.zones[strings.ToLower(name)], rr)
		}
	}
}

func (s *Server) addDefaultZone() {
	// Add sidequest.local pointing to this container's IP.
	ip := getLocalIP()
	if ip == "" {
		ip = "127.0.0.1"
	}

	rr, _ := dns.NewRR(fmt.Sprintf("sidequest.local. 300 IN A %s", ip))
	if rr != nil {
		s.zones["sidequest.local."] = append(s.zones["sidequest.local."], rr)
	}

	// Wildcard.
	wrr, _ := dns.NewRR(fmt.Sprintf("*.sidequest.local. 300 IN A %s", ip))
	if wrr != nil {
		s.zones["*.sidequest.local."] = append(s.zones["*.sidequest.local."], wrr)
	}
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	return ""
}

package dns

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func startTestServer(t *testing.T) (int, func()) {
	t.Helper()

	// Find a free port.
	l, err := net.ListenPacket("udp", ":0")
	if err != nil {
		t.Fatalf("finding free port: %v", err)
	}
	port := l.LocalAddr().(*net.UDPAddr).Port
	l.Close()

	srv := New(port)

	go srv.Start()
	time.Sleep(100 * time.Millisecond)

	return port, func() { srv.Shutdown() }
}

func queryDNS(t *testing.T, port int, name string, qtype uint16) *dns.Msg {
	t.Helper()

	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), qtype)
	m.RecursionDesired = true

	c := new(dns.Client)
	c.Timeout = 2 * time.Second

	r, _, err := c.Exchange(m, fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}
	return r
}

func TestDefaultSidequestLocalZone(t *testing.T) {
	port, cleanup := startTestServer(t)
	defer cleanup()

	r := queryDNS(t, port, "sidequest.local", dns.TypeA)

	if len(r.Answer) == 0 {
		t.Fatal("Expected answer for sidequest.local")
	}

	a, ok := r.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("Expected A record, got %T", r.Answer[0])
	}
	if a.A == nil {
		t.Error("Expected non-nil IP in A record")
	}
}

func TestDNSUpstreamForwarding(t *testing.T) {
	port, cleanup := startTestServer(t)
	defer cleanup()

	r := queryDNS(t, port, "google.com", dns.TypeA)

	if r.Rcode != dns.RcodeSuccess {
		t.Errorf("Expected success, got %s", dns.RcodeToString[r.Rcode])
	}
	if len(r.Answer) == 0 {
		t.Error("Expected answers for google.com (upstream forwarding)")
	}
}

func TestDNSUnknownDomainForwards(t *testing.T) {
	port, cleanup := startTestServer(t)
	defer cleanup()

	// Query a domain not in local zones — should forward upstream.
	r := queryDNS(t, port, "example.com", dns.TypeA)

	// Should get an answer from upstream.
	if r.Rcode != dns.RcodeSuccess {
		t.Errorf("Expected success from upstream, got %s", dns.RcodeToString[r.Rcode])
	}
}

func TestDNSServerRespondsToMultipleTypes(t *testing.T) {
	port, cleanup := startTestServer(t)
	defer cleanup()

	// Query for AAAA — should forward upstream for real domain.
	r := queryDNS(t, port, "google.com", dns.TypeAAAA)

	if r.Rcode != dns.RcodeSuccess {
		t.Errorf("Expected success for AAAA, got %s", dns.RcodeToString[r.Rcode])
	}
}

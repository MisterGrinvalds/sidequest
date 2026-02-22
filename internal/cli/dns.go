package cli

import (
	"context"
	"fmt"
	"net"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/miekg/dns"
	"github.com/spf13/cobra"

	dnsserver "github.com/MisterGrinvalds/sidequest/internal/server/dns"
)

func init() {
	rootCmd.AddCommand(dnsCmd)
	dnsCmd.AddCommand(dnsServeCmd)
	dnsCmd.AddCommand(dnsLookupCmd)
	dnsCmd.AddCommand(dnsCheckCmd)
}

var dnsCmd = &cobra.Command{
	Use:   "dns",
	Short: "DNS server and client tools",
	Long:  "Start a DNS server or perform DNS lookups and health checks.",
}

var dnsServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the DNS server",
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		srv := dnsserver.New(port)

		fmt.Printf("DNS server listening on :%d (UDP+TCP)\n", port)
		fmt.Println("Default zone: *.sidequest.local")

		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		errCh := make(chan error, 1)
		go func() {
			errCh <- srv.Start()
		}()

		select {
		case err := <-errCh:
			return err
		case <-ctx.Done():
			fmt.Println("\nShutting down...")
			srv.Shutdown()
		}
		return nil
	},
}

var dnsLookupCmd = &cobra.Command{
	Use:   "lookup <domain>",
	Short: "Perform a DNS lookup",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain := args[0]
		if !strings.HasSuffix(domain, ".") {
			domain += "."
		}

		server, _ := cmd.Flags().GetString("server")
		qtype, _ := cmd.Flags().GetString("type")
		timeout, _ := cmd.Flags().GetDuration("timeout")

		rrType, ok := dns.StringToType[strings.ToUpper(qtype)]
		if !ok {
			return fmt.Errorf("unknown record type %q", qtype)
		}

		m := new(dns.Msg)
		m.SetQuestion(domain, rrType)
		m.RecursionDesired = true

		c := new(dns.Client)
		c.Timeout = timeout

		r, rtt, err := c.Exchange(m, server)
		if err != nil {
			return fmt.Errorf("DNS query failed: %w", err)
		}

		fmt.Printf(";; Query: %s %s @%s\n", strings.TrimSuffix(domain, "."), qtype, server)
		fmt.Printf(";; Time: %s\n", rtt.Round(time.Microsecond))
		fmt.Printf(";; Status: %s, Answers: %d\n\n", dns.RcodeToString[r.Rcode], len(r.Answer))

		for _, ans := range r.Answer {
			fmt.Println(ans.String())
		}

		return nil
	},
}

var dnsCheckCmd = &cobra.Command{
	Use:   "check <domain>",
	Short: "Full DNS health check (all record types)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain := args[0]
		if !strings.HasSuffix(domain, ".") {
			domain += "."
		}

		server, _ := cmd.Flags().GetString("server")
		timeout, _ := cmd.Flags().GetDuration("timeout")

		types := []uint16{dns.TypeA, dns.TypeAAAA, dns.TypeCNAME, dns.TypeMX, dns.TypeTXT, dns.TypeNS, dns.TypeSRV, dns.TypeSOA}
		typeNames := []string{"A", "AAAA", "CNAME", "MX", "TXT", "NS", "SRV", "SOA"}

		fmt.Printf("DNS Health Check: %s (server: %s)\n", strings.TrimSuffix(domain, "."), server)
		fmt.Println(strings.Repeat("-", 60))

		c := new(dns.Client)
		c.Timeout = timeout

		for i, t := range types {
			m := new(dns.Msg)
			m.SetQuestion(domain, t)
			m.RecursionDesired = true

			r, rtt, err := c.Exchange(m, server)
			if err != nil {
				fmt.Printf("%-8s ERROR: %v\n", typeNames[i], err)
				continue
			}

			if len(r.Answer) == 0 {
				fmt.Printf("%-8s (no records) [%s]\n", typeNames[i], rtt.Round(time.Microsecond))
			} else {
				for _, ans := range r.Answer {
					fmt.Printf("%-8s %s [%s]\n", typeNames[i], ans.String(), rtt.Round(time.Microsecond))
				}
			}
		}

		// Reverse lookup.
		fmt.Println()
		fmt.Println("Resolving IPs:")
		ips, err := net.LookupIP(strings.TrimSuffix(domain, "."))
		if err == nil {
			for _, ip := range ips {
				names, _ := net.LookupAddr(ip.String())
				fmt.Printf("  %s -> %s\n", ip, strings.Join(names, ", "))
			}
		}

		return nil
	},
}

func init() {
	dnsServeCmd.Flags().Int("port", 5353, "Port to listen on")

	for _, cmd := range []*cobra.Command{dnsLookupCmd, dnsCheckCmd} {
		cmd.Flags().String("server", "8.8.8.8:53", "DNS server to query")
		cmd.Flags().Duration("timeout", 5*time.Second, "Query timeout")
	}
	dnsLookupCmd.Flags().String("type", "A", "Record type (A, AAAA, CNAME, MX, TXT, NS, SRV)")
}

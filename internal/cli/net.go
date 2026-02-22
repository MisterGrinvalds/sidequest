package cli

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(netCmd)
	netCmd.AddCommand(netPingCmd)
	netCmd.AddCommand(netTraceCmd)
	netCmd.AddCommand(netPortsCmd)
	netCmd.AddCommand(netInterfacesCmd)
}

var netCmd = &cobra.Command{
	Use:   "net",
	Short: "Network diagnostic tools",
	Long:  "Ping, traceroute, port scanning, and network interface inspection.",
}

var netPingCmd = &cobra.Command{
	Use:   "ping <host>",
	Short: "Ping a host",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		host := args[0]
		count, _ := cmd.Flags().GetInt("count")

		// Use system ping for maximum compatibility and ICMP support.
		pingArgs := []string{"-c", fmt.Sprintf("%d", count), host}
		c := exec.CommandContext(context.Background(), "ping", pingArgs...)
		c.Stdout = cmd.OutOrStdout()
		c.Stderr = cmd.OutOrStderr()
		return c.Run()
	},
}

var netTraceCmd = &cobra.Command{
	Use:   "trace <host>",
	Short: "Traceroute to a host",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		host := args[0]
		maxHops, _ := cmd.Flags().GetInt("max-hops")

		// Try mtr first (better output), fall back to traceroute.
		if path, err := exec.LookPath("mtr"); err == nil {
			c := exec.CommandContext(context.Background(), path, "--report", "--report-cycles", "1", "-m", fmt.Sprintf("%d", maxHops), host)
			c.Stdout = cmd.OutOrStdout()
			c.Stderr = cmd.OutOrStderr()
			return c.Run()
		}

		if path, err := exec.LookPath("traceroute"); err == nil {
			c := exec.CommandContext(context.Background(), path, "-m", fmt.Sprintf("%d", maxHops), host)
			c.Stdout = cmd.OutOrStdout()
			c.Stderr = cmd.OutOrStderr()
			return c.Run()
		}

		return fmt.Errorf("neither mtr nor traceroute found in PATH")
	},
}

var netPortsCmd = &cobra.Command{
	Use:   "ports <host>",
	Short: "Scan common ports on a host",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		host := args[0]
		portRange, _ := cmd.Flags().GetString("range")
		timeout, _ := cmd.Flags().GetDuration("timeout")

		ports := parsePortRange(portRange)
		if len(ports) == 0 {
			// Common ports.
			ports = []int{21, 22, 23, 25, 53, 80, 110, 143, 443, 993, 995, 3306, 5432, 6379, 8080, 8443, 9090, 27017}
		}

		fmt.Printf("Scanning %s (%d ports)...\n\n", host, len(ports))

		open := 0
		for _, port := range ports {
			addr := fmt.Sprintf("%s:%d", host, port)
			conn, err := net.DialTimeout("tcp", addr, timeout)
			if err == nil {
				conn.Close()
				fmt.Printf("  %-6d OPEN\n", port)
				open++
			}
		}

		if open == 0 {
			fmt.Println("  (no open ports found)")
		}
		fmt.Printf("\n%d open ports found\n", open)
		return nil
	},
}

var netInterfacesCmd = &cobra.Command{
	Use:   "interfaces",
	Short: "Show network interfaces, IPs, and routes",
	RunE: func(cmd *cobra.Command, args []string) error {
		ifaces, err := net.Interfaces()
		if err != nil {
			return fmt.Errorf("listing interfaces: %w", err)
		}

		fmt.Println("Network Interfaces:")
		fmt.Println(strings.Repeat("-", 60))

		for _, iface := range ifaces {
			addrs, _ := iface.Addrs()
			flags := iface.Flags.String()

			fmt.Printf("%-15s MTU:%-6d Flags: %s\n", iface.Name, iface.MTU, flags)
			if iface.HardwareAddr != nil {
				fmt.Printf("  %-13s MAC: %s\n", "", iface.HardwareAddr)
			}
			for _, addr := range addrs {
				fmt.Printf("  %-13s %s\n", "", addr.String())
			}
			fmt.Println()
		}

		// Show routes if ip command is available.
		if path, err := exec.LookPath("ip"); err == nil {
			fmt.Println("Routes:")
			fmt.Println(strings.Repeat("-", 60))
			c := exec.CommandContext(context.Background(), path, "route", "show")
			c.Stdout = cmd.OutOrStdout()
			c.Stderr = cmd.OutOrStderr()
			c.Run()
		}

		return nil
	},
}

func init() {
	netPingCmd.Flags().Int("count", 4, "Number of pings")
	netTraceCmd.Flags().Int("max-hops", 30, "Maximum number of hops")
	netPortsCmd.Flags().String("range", "", "Port range (e.g. 1-1024)")
	netPortsCmd.Flags().Duration("timeout", 2*time.Second, "Connection timeout per port")
}

func parsePortRange(s string) []int {
	if s == "" {
		return nil
	}
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return nil
	}
	var start, end int
	fmt.Sscanf(parts[0], "%d", &start)
	fmt.Sscanf(parts[1], "%d", &end)
	if start <= 0 || end <= 0 || start > end || end > 65535 {
		return nil
	}
	ports := make([]int, 0, end-start+1)
	for i := start; i <= end; i++ {
		ports = append(ports, i)
	}
	return ports
}

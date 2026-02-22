package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(containerCmd)
	containerCmd.AddCommand(containerRuntimeCmd)
	containerCmd.AddCommand(containerInspectCmd)
	containerCmd.AddCommand(containerPsCmd)
}

var containerCmd = &cobra.Command{
	Use:   "container",
	Short: "Container runtime inspection tools",
	Long:  "Detect container runtime, inspect cgroups/namespaces/capabilities.",
}

var containerRuntimeCmd = &cobra.Command{
	Use:   "runtime",
	Short: "Detect the container runtime",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Container Runtime Detection:")
		fmt.Println(strings.Repeat("-", 40))

		// Check for Docker.
		if _, err := os.Stat("/.dockerenv"); err == nil {
			fmt.Println("  Docker:      YES (/.dockerenv found)")
		} else {
			fmt.Println("  Docker:      no")
		}

		// Check for Podman.
		if _, err := os.Stat("/run/.containerenv"); err == nil {
			fmt.Println("  Podman:      YES (/run/.containerenv found)")
		} else {
			fmt.Println("  Podman:      no")
		}

		// Check for Kubernetes.
		if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
			fmt.Printf("  Kubernetes:  YES (API: %s:%s)\n",
				os.Getenv("KUBERNETES_SERVICE_HOST"),
				os.Getenv("KUBERNETES_SERVICE_PORT"))
		} else {
			fmt.Println("  Kubernetes:  no")
		}

		// Check cgroup for runtime hints.
		if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
			content := string(data)
			if strings.Contains(content, "containerd") {
				fmt.Println("  containerd:  YES (cgroup hint)")
			}
			if strings.Contains(content, "crio") || strings.Contains(content, "cri-o") {
				fmt.Println("  CRI-O:       YES (cgroup hint)")
			}
		}

		// Check for systemd container env.
		if data, err := os.ReadFile("/run/systemd/container"); err == nil {
			fmt.Printf("  Systemd:     %s\n", strings.TrimSpace(string(data)))
		}

		return nil
	},
}

var containerInspectCmd = &cobra.Command{
	Use:   "inspect",
	Short: "Inspect current container (cgroups, namespaces, capabilities)",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Container Inspection:")
		fmt.Println(strings.Repeat("=", 60))

		// cgroups.
		fmt.Println("\nCgroups (v2):")
		fmt.Println(strings.Repeat("-", 40))
		cgroupPaths := []struct{ label, path string }{
			{"Memory Limit", "/sys/fs/cgroup/memory.max"},
			{"Memory Usage", "/sys/fs/cgroup/memory.current"},
			{"CPU Max", "/sys/fs/cgroup/cpu.max"},
			{"CPU Weight", "/sys/fs/cgroup/cpu.weight"},
			{"PIDs Max", "/sys/fs/cgroup/pids.max"},
			{"PIDs Current", "/sys/fs/cgroup/pids.current"},
		}
		for _, cg := range cgroupPaths {
			if data, err := os.ReadFile(cg.path); err == nil {
				fmt.Printf("  %-16s %s", cg.label+":", strings.TrimSpace(string(data)))
				fmt.Println()
			}
		}

		// Fallback: cgroups v1.
		cgroupV1Paths := []struct{ label, path string }{
			{"Memory Limit", "/sys/fs/cgroup/memory/memory.limit_in_bytes"},
			{"Memory Usage", "/sys/fs/cgroup/memory/memory.usage_in_bytes"},
			{"CPU Quota", "/sys/fs/cgroup/cpu/cpu.cfs_quota_us"},
			{"CPU Period", "/sys/fs/cgroup/cpu/cpu.cfs_period_us"},
		}
		v1Found := false
		for _, cg := range cgroupV1Paths {
			if data, err := os.ReadFile(cg.path); err == nil {
				if !v1Found {
					fmt.Println("\nCgroups (v1):")
					fmt.Println(strings.Repeat("-", 40))
					v1Found = true
				}
				fmt.Printf("  %-16s %s\n", cg.label+":", strings.TrimSpace(string(data)))
			}
		}

		// Capabilities.
		fmt.Println("\nCapabilities:")
		fmt.Println(strings.Repeat("-", 40))
		capPaths := []struct{ label, path string }{
			{"Effective", "/proc/self/status"},
		}
		for _, cp := range capPaths {
			if data, err := os.ReadFile(cp.path); err == nil {
				for _, line := range strings.Split(string(data), "\n") {
					if strings.HasPrefix(line, "Cap") {
						fmt.Printf("  %s\n", line)
					}
				}
			}
		}

		// Namespaces.
		fmt.Println("\nNamespaces:")
		fmt.Println(strings.Repeat("-", 40))
		if entries, err := os.ReadDir("/proc/self/ns"); err == nil {
			for _, e := range entries {
				if target, err := os.Readlink(fmt.Sprintf("/proc/self/ns/%s", e.Name())); err == nil {
					fmt.Printf("  %-12s %s\n", e.Name()+":", target)
				}
			}
		}

		return nil
	},
}

var containerPsCmd = &cobra.Command{
	Use:   "ps",
	Short: "List processes in the container",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Use ps if available.
		if psPath, err := exec.LookPath("ps"); err == nil {
			c := exec.Command(psPath, "aux")
			c.Stdout = cmd.OutOrStdout()
			c.Stderr = cmd.OutOrStderr()
			return c.Run()
		}

		// Fallback: read /proc.
		entries, err := os.ReadDir("/proc")
		if err != nil {
			return fmt.Errorf("reading /proc: %w", err)
		}

		fmt.Printf("%-8s %-20s %s\n", "PID", "NAME", "CMDLINE")
		fmt.Println(strings.Repeat("-", 60))

		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			pid := e.Name()
			// Check if directory name is a number.
			isNum := true
			for _, c := range pid {
				if c < '0' || c > '9' {
					isNum = false
					break
				}
			}
			if !isNum {
				continue
			}

			comm, _ := os.ReadFile(fmt.Sprintf("/proc/%s/comm", pid))
			cmdline, _ := os.ReadFile(fmt.Sprintf("/proc/%s/cmdline", pid))
			cmdStr := strings.ReplaceAll(string(cmdline), "\x00", " ")
			fmt.Printf("%-8s %-20s %s\n", pid, strings.TrimSpace(string(comm)), strings.TrimSpace(cmdStr))
		}
		return nil
	},
}

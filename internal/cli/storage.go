package cli

import (
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(storageCmd)
	storageCmd.AddCommand(storageDfCmd)
	storageCmd.AddCommand(storageMountsCmd)
	storageCmd.AddCommand(storageIOTestCmd)
}

var storageCmd = &cobra.Command{
	Use:   "storage",
	Short: "Storage inspection and benchmarking tools",
	Long:  "Inspect disk usage, mounts, and run I/O benchmarks.",
}

var storageDfCmd = &cobra.Command{
	Use:   "df",
	Short: "Show disk usage (filesystem info)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if dfPath, err := exec.LookPath("df"); err == nil {
			c := exec.Command(dfPath, "-h")
			c.Stdout = cmd.OutOrStdout()
			c.Stderr = cmd.OutOrStderr()
			return c.Run()
		}
		return fmt.Errorf("df not found in PATH")
	},
}

var storageMountsCmd = &cobra.Command{
	Use:   "mounts",
	Short: "Show mounted volumes",
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := os.ReadFile("/proc/mounts")
		if err != nil {
			// Fallback to mount command.
			if mountPath, err := exec.LookPath("mount"); err == nil {
				c := exec.Command(mountPath)
				c.Stdout = cmd.OutOrStdout()
				c.Stderr = cmd.OutOrStderr()
				return c.Run()
			}
			return fmt.Errorf("cannot read mounts: %w", err)
		}

		fmt.Printf("%-30s %-20s %-10s %s\n", "DEVICE", "MOUNTPOINT", "TYPE", "OPTIONS")
		fmt.Println(strings.Repeat("-", 90))

		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 4 {
				continue
			}
			device := fields[0]
			mountpoint := fields[1]
			fstype := fields[2]
			options := fields[3]

			// Skip noise filesystems.
			if strings.HasPrefix(fstype, "proc") || fstype == "sysfs" || fstype == "cgroup" || fstype == "cgroup2" {
				continue
			}

			fmt.Printf("%-30s %-20s %-10s %s\n", device, mountpoint, fstype, truncate(options, 30))
		}
		return nil
	},
}

var storageIOTestCmd = &cobra.Command{
	Use:   "io-test",
	Short: "Run a simple I/O benchmark",
	RunE: func(cmd *cobra.Command, args []string) error {
		path, _ := cmd.Flags().GetString("path")
		sizeMB, _ := cmd.Flags().GetInt("size")

		testFile := fmt.Sprintf("%s/.sidequest-io-test-%d", path, time.Now().UnixNano())
		defer os.Remove(testFile)

		data := make([]byte, 1024*1024) // 1MB block
		rand.Read(data)

		// Write test.
		fmt.Printf("I/O Benchmark: %s (%d MB)\n", path, sizeMB)
		fmt.Println(strings.Repeat("-", 40))

		start := time.Now()
		f, err := os.Create(testFile)
		if err != nil {
			return fmt.Errorf("creating test file: %w", err)
		}
		for i := 0; i < sizeMB; i++ {
			if _, err := f.Write(data); err != nil {
				f.Close()
				return fmt.Errorf("writing: %w", err)
			}
		}
		f.Sync()
		f.Close()
		writeTime := time.Since(start)
		writeMBps := float64(sizeMB) / writeTime.Seconds()

		// Read test.
		start = time.Now()
		f, err = os.Open(testFile)
		if err != nil {
			return fmt.Errorf("opening test file: %w", err)
		}
		buf := make([]byte, 1024*1024)
		for {
			_, err := f.Read(buf)
			if err != nil {
				break
			}
		}
		f.Close()
		readTime := time.Since(start)
		readMBps := float64(sizeMB) / readTime.Seconds()

		fmt.Printf("  Write: %.1f MB/s (%d MB in %s)\n", writeMBps, sizeMB, writeTime.Round(time.Millisecond))
		fmt.Printf("  Read:  %.1f MB/s (%d MB in %s)\n", readMBps, sizeMB, readTime.Round(time.Millisecond))

		return nil
	},
}

func init() {
	storageIOTestCmd.Flags().String("path", "/tmp", "Directory to run benchmark in")
	storageIOTestCmd.Flags().Int("size", 100, "Test size in MB")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

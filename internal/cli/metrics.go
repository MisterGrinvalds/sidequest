package cli

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(metricsCmd)
}

var metricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "Launch btop++ system metrics dashboard",
	Long: `Launch the btop++ terminal dashboard for system metrics.

btop++ must be installed (included in the sidequest container image).
Install locally with: brew install btop (macOS) or apk add btop (Alpine).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tty, _ := cmd.Flags().GetBool("tty")

		btopPath, err := exec.LookPath("btop")
		if err != nil {
			fmt.Fprintln(os.Stderr, "btop++ not found in PATH.")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "Install with:")
			fmt.Fprintln(os.Stderr, "  macOS:   brew install btop")
			fmt.Fprintln(os.Stderr, "  Alpine:  apk add btop")
			fmt.Fprintln(os.Stderr, "  Ubuntu:  apt install btop")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "btop++ is pre-installed in the sidequest container image.")
			return fmt.Errorf("btop++ not found")
		}

		btopArgs := []string{"btop"}
		if tty {
			btopArgs = append(btopArgs, "--tty")
		}

		// Replace the current process with btop.
		return syscall.Exec(btopPath, btopArgs, os.Environ())
	},
}

func init() {
	metricsCmd.Flags().Bool("tty", false, "Use TTY mode (16 colors, simpler characters)")
}

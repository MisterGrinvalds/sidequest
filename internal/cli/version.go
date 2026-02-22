package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	buildVersion = "dev"
	buildCommit  = "unknown"
	buildDate    = "unknown"
)

// SetBuildInfo sets the build-time version information.
func SetBuildInfo(version, commit, date string) {
	buildVersion = version
	buildCommit = commit
	buildDate = date
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version and build information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("sidequest %s\n", buildVersion)
		fmt.Printf("  commit:  %s\n", buildCommit)
		fmt.Printf("  built:   %s\n", buildDate)
		fmt.Printf("  go:      %s\n", runtime.Version())
		fmt.Printf("  os/arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	},
}

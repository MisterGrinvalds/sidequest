package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/MisterGrinvalds/sidequest/internal/config"
)

var cfg *config.Config

var rootCmd = &cobra.Command{
	Use:   "sidequest",
	Short: "A multi-tool Swiss Army knife for Kubernetes and container runtimes",
	Long: `Sidequest is a multi-tool container that provides HTTP, REST, gRPC, GraphQL,
and DNS servers, along with network diagnostics, container inspection, storage
tools, identity provider capabilities, and Kubernetes utilities.

All packed into a single binary and Alpine-based container image.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		cfg = config.Load()
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().String("log-level", "info", "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().String("log-format", "text", "Log format (text, json)")
}

// Execute runs the root command.
func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}
	return nil
}

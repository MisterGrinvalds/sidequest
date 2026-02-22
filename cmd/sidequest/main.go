package main

import (
	"os"

	"github.com/MisterGrinvalds/sidequest/internal/cli"
)

// Build-time variables set via ldflags.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	cli.SetBuildInfo(version, commit, date)
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}

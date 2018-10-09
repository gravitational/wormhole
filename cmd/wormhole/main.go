package main

import (
	"os"

	"github.com/gravitational/trace"
	"github.com/spf13/cobra"
)

var (
	// Build time variables
	commitHash string
	timestamp  string
	gitTag     string
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Stderr.Write([]byte(trace.DebugReport(err)))
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "wormhole",
	Short: "",
	Long:  ``,
}

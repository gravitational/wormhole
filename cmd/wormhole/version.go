package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/gravitational/trace"
	"github.com/spf13/cobra"
	yaml "gopkg.in/yaml.v2"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "",
	Long:  ``,
	RunE:  version,
}

var (
	outputFormat = "text"
)

func init() {
	rootCmd.AddCommand(versionCmd)
	versionCmd.Flags().StringVarP(&outputFormat, "output", "o", outputFormat, "Output format. One Of: text|json|yaml")
}

func version(cmd *cobra.Command, args []string) error {

	kv := map[string]string{
		"version":   gitTag,
		"hash":      commitHash,
		"timestamp": timestamp,
	}

	switch strings.ToLower(outputFormat) {
	case "text":
		fmt.Println("Version:         ", gitTag)
		fmt.Println("Hash:            ", commitHash)
		fmt.Println("Build Timestamp: ", timestamp)
	case "json":
		b, err := json.MarshalIndent(kv, "", "  ")
		if err != nil {
			return trace.ConvertSystemError(err)
		}
		os.Stdout.Write(b)
	case "yaml":
		b, err := yaml.Marshal(kv)
		if err != nil {
			return trace.ConvertSystemError(err)
		}
		os.Stdout.Write(b)
	}

	return nil
}

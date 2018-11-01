/*
Copyright 2018 Gravitational, Inc.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
	Short: "Print version information.",
	Long:  ``,
	RunE:  version,
}

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

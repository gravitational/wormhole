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
	"os"

	"github.com/sirupsen/logrus"

	"github.com/gravitational/trace"
	"github.com/spf13/cobra"
)

var (
	// Build time variables
	commitHash string
	timestamp  string
	gitTag     string
)

var (
	outputFormat = "text"
	debug        = false
	logger       = logrus.New()
)

func init() {
	rootCmd.Flags().BoolVarP(
		&debug,
		"debug",
		"",
		debug,
		"Enable debug logging",
	)
}

func main() {
	if debug {
		logger.SetLevel(logrus.DebugLevel)
	}

	if err := rootCmd.Execute(); err != nil {
		os.Stderr.Write([]byte(trace.DebugReport(err)))
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use: "wormhole",
}

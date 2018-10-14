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
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"

	"github.com/gravitational/trace"
	"github.com/gravitational/wormhole/pkg/daemon"
	"github.com/spf13/cobra"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run the main daemon process",
	Long:  ``,
	RunE:  runDaemon,
}

func init() {
	rootCmd.AddCommand(daemonCmd)
	daemonCmd.Flags().StringVarP(
		&kubeletKubeconfig,
		"kubeconfig-kubelet",
		"",
		kubeletKubeconfig,
		"kubelet kubeconfig file",
	)
	daemonCmd.Flags().StringVarP(
		&wormholeKubeconfig,
		"kubeconfig-wormhole",
		"",
		wormholeKubeconfig,
		"wormhole kubeconfig file",
	)
	daemonCmd.Flags().StringVarP(
		&nodeName,
		"node-name",
		"n",
		nodeName,
		"the name of the k8s node this instance is running on",
	)
	daemonCmd.Flags().StringVarP(
		&overlayCIDR,
		"overlay-cidr",
		"",
		overlayCIDR,
		"The cidr for the overlay network",
	)
	daemonCmd.Flags().Uint16VarP(
		&port,
		"port",
		"",
		port,
		"The external port to use for wireguard connections",
	)
}

var (
	kubeletKubeconfig  string
	wormholeKubeconfig string
	nodeName           string
	overlayCIDR        string
	port               uint16 = 9086
)

func runDaemon(cmd *cobra.Command, args []string) error {
	daemon := &daemon.Daemon{
		FieldLogger: logrus.New(),
		NodeName:    nodeName,
		OverlayCIDR: overlayCIDR,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := daemon.Run(ctx, kubeletKubeconfig, wormholeKubeconfig)
	if err != nil {
		return trace.Wrap(err)
	}

	signalC := make(chan os.Signal, 2)
	signal.Notify(signalC, os.Interrupt, syscall.SIGTERM)

	<-signalC
	return nil
}

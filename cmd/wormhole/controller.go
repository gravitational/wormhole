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

	"github.com/magefile/mage/sh"

	"github.com/gravitational/trace"
	"github.com/gravitational/wormhole/pkg/controller"
	"github.com/spf13/cobra"
)

var controllerCmd = &cobra.Command{
	Use:   "controller",
	Short: "Controller for setting up wireguard overlay network between hosts",
	Long: `
Run the main control loop and setup the wireguard overlay network.

The controller can run either on a system when passed kubeconfig settings, or as a pod within the cluster.
	`,
	RunE: runController,
}

func init() {
	rootCmd.AddCommand(controllerCmd)
	controllerCmd.Flags().StringVarP(
		&kubeconfigPath,
		"kubeconfig",
		"",
		kubeconfigPath,
		"Path to kubeconfig file for controller to interact with kubernetes",
	)
	controllerCmd.Flags().StringVarP(
		&nodeName,
		"node-name",
		"n",
		nodeName,
		"the name of the k8s node this instance is running on",
	)
	controllerCmd.Flags().StringVarP(
		&overlayCIDR,
		"overlay-cidr",
		"",
		overlayCIDR,
		"The cidr assigned for the overlay network (each pod subnet must exist within the overlay)",
	)
	controllerCmd.Flags().StringVarP(
		&nodeCIDR,
		"node-cidr",
		"",
		nodeCIDR,
		"The cidr assigned to this node",
	)
	controllerCmd.Flags().IntVarP(
		&port,
		"port",
		"",
		port,
		"The external port to use for wireguard connections (default 9806)",
	)
	controllerCmd.Flags().StringVarP(
		&wireguardIface,
		"wireguard-iface",
		"",
		wireguardIface,
		"The name of the wireguard interface to create (default wormhole-wg0)",
	)
	controllerCmd.Flags().StringVarP(
		&bridgeIface,
		"bridge-iface",
		"",
		bridgeIface,
		"The name of the internal bridge to create (default wormhole-br0)",
	)
}

var (
	kubeconfigPath string
	nodeName       string
	overlayCIDR    string
	nodeCIDR       string
	endpoint       string
	port           = 9806
	wireguardIface = "wormhole-wg0"
	bridgeIface    = "wormhole-br0"
	namespace      = "wormhole"
)

func runController(cmd *cobra.Command, args []string) error {
	err := syncCniBin()
	if err != nil {
		return trace.Wrap(err)
	}

	c, err := controller.New(controller.Config{
		NodeName:       nodeName,
		Namespace:      namespace,
		OverlayCIDR:    overlayCIDR,
		NodeCIDR:       nodeCIDR,
		Port:           port,
		WireguardIface: wireguardIface,
		BridgeIface:    bridgeIface,
		KubeconfigPath: kubeconfigPath,
		Endpoint:       endpoint,
	})
	if err != nil {
		return trace.Wrap(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = c.Run(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	signalC := make(chan os.Signal, 2)
	signal.Notify(signalC, os.Interrupt, syscall.SIGTERM)

	<-signalC
	return nil
}

// syncCniBin tries to detect if we have a host mounted cni bin, and if we do, copy the cni binaries to the host
func syncCniBin() error {
	if _, err := os.Stat("/host/opt/cni/bin"); !os.IsNotExist(err) {
		err = sh.Run("bash", "-c", "cp /opt/cni/bin/* /host/opt/cni/bin/")
		if err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

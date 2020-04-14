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
	controllerCmd.Flags().StringVarP(
		&endpoint,
		"endpoint",
		"",
		endpoint,
		"The endpoint to use for wireguard connections (detected by default from kubernetes node object)",
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
	controllerCmd.Flags().BoolVarP(
		&debug,
		"debug",
		"",
		debug,
		"Enable debug logging",
	)
	controllerCmd.Flags().IntVarP(
		&bridgeMTU,
		"bridge-mtu",
		"",
		bridgeMTU,
		"The MTU value to assign to the internal linux bridge",
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

	// TODO(knisbet)
	// Investigate what MTU setting to use. There are a few things to consider:
	//   - 65535 is the maximum mtu that can be set on a bridge
	//   - This depends significantly, on how the linux kernel represents packets as they pass between
	//     network namespaces and through the linux bridge. If they're represented as ethernet packets,
	//     a large mtu should allow pod-to-pod within a host to be more efficient
	//   - Wireguard implements its own segmentation, and indicates to the linux kernel that it supports
	//     generic segmentation offload (https://www.wireguard.com/papers/wireguard.pdf section 7.1). If
	//     the bridge MTU plays into this, again, having a large mtu should be more efficient for pod-to-pod
	//     traffic between hosts.
	//   - If the network driver supports/has segmentation offload enabled, having large internal frames
	//     should also be more efficient. So pod -> internet traffic is segmented by the nic if enabled.
	//   - Also need to check into, whether we're getting a correct MSS, all of this is wasted if we're
	//     using a standard MSS in the TCP handshake
	//   - Also need to check whether we're advertising too large of a MSS on our TCP connections to internet
	//     peers, which may cause traffic towards a pod to have PMTU/black hole problems
	bridgeMTU = 65535
)

func runController(cmd *cobra.Command, args []string) error {
	err := syncCniBin()
	if err != nil {
		return trace.Wrap(err)
	}

	logger := logrus.New()
	if debug {
		logger.SetLevel(logrus.DebugLevel)
	}

	c, err := controller.New(controller.Config{
		NodeName:       nodeName,
		Namespace:      namespace,
		OverlayCIDR:    overlayCIDR,
		NodeCIDR:       nodeCIDR,
		ListenPort:     port,
		WireguardIface: wireguardIface,
		BridgeIface:    bridgeIface,
		BridgeMTU:      bridgeMTU,
		KubeconfigPath: kubeconfigPath,
		Endpoint:       endpoint,
	}, logger)
	if err != nil {
		return trace.Wrap(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		signalC := make(chan os.Signal, 1)
		signal.Notify(signalC, os.Interrupt, syscall.SIGTERM)

		<-signalC
		cancel()
	}()

	err = c.Run(ctx)
	if err != nil && trace.Unwrap(err) != context.Canceled {
		return trace.Wrap(err)
	}

	return nil
}

// syncCniBin attempts to copy CNI plugins to the host
// When running as a container, the host /opt/cni/bin directory should be mounted under /host
// If the /host/opt/cni/bin directory exists, copy the plugins to the host
func syncCniBin() error {
	if _, err := os.Stat("/host/opt/cni/bin"); !os.IsNotExist(err) {
		err = sh.Run("bash", "-c", "chown root:root -R /host/opt/cni/bin && cp /opt/cni/bin/* /host/opt/cni/bin/")
		if err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

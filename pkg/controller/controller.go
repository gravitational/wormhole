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

package controller

import (
	"context"
	"os"

	"github.com/davecgh/go-spew/spew"

	"github.com/gravitational/wormhole/pkg/iptables"
	"github.com/gravitational/wormhole/pkg/wireguard"

	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

type Controller struct {
	logrus.FieldLogger

	// NodeName is the name of the k8s node this instance is running on
	NodeName string

	// OverlayCIDR is the IP network CIDR for the entire overlay network
	OverlayCIDR string

	// Port is the external port wireguard should listen on for tunnels
	Port int

	// WireguardIface is the name of the wireguard interface to create
	WireguardIface string

	// BridgeIface is the name of the bridge to create
	BridgeIface string

	// nodeClient is a kubernetes client for accessing the nodes own object
	nodeClient *kubernetes.Clientset
	// wireguardClient is a kubernetes client for accessing wireguard related config
	wormholeClient *kubernetes.Clientset

	// publicKey is the public key of this node
	publicKey string
	// sharedKey is the shared key of this cluster
	sharedKey string

	// nodePodCIDR is the cidr range for pods on this host from IPAM
	nodePodCIDR string
	// podGateway is the gateway address to use for pods
	podGateway string
	// wormholeGateway is the gateway for overlay traffic
	wormholeGateway string
	// podRangeStart is the start of the ip range to assign to pods
	podRangeStart string
	// podRangeEnd is the end of the ip range to assign to pods
	podRangeEnd string

	nodeController cache.Controller
	nodeList       listers.NodeLister
}

func (d *Controller) init(nodeKubeconfig, wormKubeconfig string) error {
	var err error
	d.nodeClient, err = getClientsetFromKubeconfig(nodeKubeconfig)
	if err != nil {
		return trace.Wrap(err)
	}

	if wormKubeconfig != "" {
		d.wormholeClient, err = getClientsetFromKubeconfig(wormKubeconfig)
		if err != nil {
			return trace.Wrap(err)
		}
	} else {
		clusterConfig, err := rest.InClusterConfig()
		if err != nil {
			return trace.Wrap(err)
		}
		d.wormholeClient, err = kubernetes.NewForConfig(clusterConfig)
		if err != nil {
			return trace.Wrap(err)
		}
	}

	return nil
}

func runningInPod() bool {
	return os.Getenv("POD_NAME") != ""
}

func getClientsetFromKubeconfig(path string) (*kubernetes.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return clientset, nil
}

func (d *Controller) Run(ctx context.Context, nodeKubeconfig, wormKubeconfig string) error {
	d.Info("Starting wormhole Controller with config: ", spew.Sdump(d))

	err := d.init(nodeKubeconfig, wormKubeconfig)
	if err != nil {
		return trace.Wrap(err)
	}

	err = d.startup(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	iptablesSync := iptables.Config{
		FieldLogger:    d.FieldLogger.WithField("module", "iptables"),
		OverlayCIDR:    d.OverlayCIDR,
		PodCIDR:        d.nodePodCIDR,
		WireguardIface: d.WireguardIface,
		BridgeIface:    d.BridgeIface,
	}
	err = iptablesSync.Run(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

func (d *Controller) startup(ctx context.Context) error {
	d.Info("Starting Wormhole...")
	var err error
	if d.NodeName == "" {
		err = d.detectNodeName()
		if err != nil {
			return trace.Wrap(err)
		}
	}

	psk, err := d.getOrSetPSK()
	if err != nil {
		return trace.Wrap(err)
	}
	d.sharedKey = psk
	// TODO(knisbet) security!!! remove logging of secrets
	d.Info("PSK: ", psk)

	pubKey, privKey, err := d.generateKeypair()
	if err != nil {
		return trace.Wrap(err)
	}
	d.publicKey = pubKey
	// TODO(knisbet) security!!! remove logging of secrets
	d.Info("PubKey: ", pubKey)
	d.Info("PrivKey: ", privKey)

	// send our new public key to the cluster
	err = d.publishPublicKey()
	if err != nil {
		return trace.Wrap(err)
	}

	// retrieve this instances cidr from the k8s IPAM
	err = d.getPodCIDR()
	if err != nil {
		return trace.Wrap(err)
	}

	// configure wireguard
	// TODO(knisbet) right now, I'm just ignoring errors in the setup
	// but this needs to eventually be handled properly
	_ = wireguard.CreateInterface(d.WireguardIface)
	_ = wireguard.SetIP(d.WireguardIface, d.wormholeGateway)
	_ = wireguard.SetPrivateKey(d.WireguardIface, privKey)
	_ = wireguard.SetListenPort(d.WireguardIface, d.Port)
	_ = wireguard.SetUp(d.WireguardIface)

	if d.OverlayCIDR == "" {
		// TODO (implement reading overlay cidr from network)
		return trace.NotImplemented("OverlayCIDR detection not implemented")
	}
	_ = wireguard.SetRoute(d.WireguardIface, d.OverlayCIDR)

	// configure CNI on the host
	err = d.configureCNI()
	if err != nil {
		return trace.Wrap(err)
	}

	kubernetesSync := kubernetesSync{
		Controller: d,
	}
	kubernetesSync.start(ctx)

	wireguardSync := wireguardSync{
		Controller: d,
	}
	err = wireguardSync.start(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

func (d *Controller) detectNodeName() error {
	d.Debug("Attempting to detect nodename.")
	defer func() { d.Info("Detected hostname: ", d.NodeName) }()
	// if we're running inside a pod
	// find the node the pod is assigned to
	podName := os.Getenv("POD_NAME")
	podNamespace := os.Getenv("POD_NAMESPACE")
	if podName != "" && podNamespace != "" {
		return trace.Wrap(d.updateNodeNameFromPod(podName, podNamespace))
	}

	nodeName, err := os.Hostname()
	if err != nil {
		return trace.Wrap(err)
	}
	// TODO(knisbet) we should probably validate here, a node object exists that matches our node name
	d.NodeName = nodeName
	return nil
}

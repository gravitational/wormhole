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

package daemon

import (
	"context"

	"github.com/gravitational/wormhole/pkg/iptables"
	"github.com/gravitational/wormhole/pkg/wireguard"

	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	DefaultWireguardIface = "wormhole0"
)

type Daemon struct {
	logrus.FieldLogger

	// NodeName is the name of the k8s node this instance is running on
	NodeName string

	// OverlayCIDR is the IP network CIDR for the entire overlay network
	OverlayCIDR string

	// nodeClient is a kubernetes client for accessing the nodes own object
	nodeClient *kubernetes.Clientset
	// wireguardClient is a kubernetes client for accessing wireguard related config
	wormholeClient *kubernetes.Clientset

	publicKey   string
	sharedKey   string
	nodePodCIDR string

	podGateway      string
	wormholeGateway string
	podRangeStart   string
	podRangeEnd     string
	// Iface is the wireguard interface name
	Iface string

	// kubernetes controller/informer
	controller cache.Controller
	cache      cache.Store

	nodeController cache.Controller
	nodeList       listers.NodeLister
}

func (d *Daemon) init(nodeKubeconfig, wormKubeconfig string) error {
	var err error
	d.nodeClient, err = getClientsetFromKubeconfig(nodeKubeconfig)
	if err != nil {
		return trace.Wrap(err)
	}

	d.wormholeClient, err = getClientsetFromKubeconfig(wormKubeconfig)
	if err != nil {
		return trace.Wrap(err)
	}

	if d.Iface == "" {
		d.Iface = DefaultWireguardIface
	}
	return nil
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

func (d *Daemon) Run(ctx context.Context, nodeKubeconfig, wormKubeconfig string) error {
	err := d.init(nodeKubeconfig, wormKubeconfig)
	if err != nil {
		return trace.Wrap(err)
	}

	err = d.startup(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	iptablesSync := iptables.Config{
		FieldLogger: d.FieldLogger.WithField("module", "iptables"),
		OverlayCIDR: d.OverlayCIDR,
		PodCIDR:     d.nodePodCIDR,
	}
	err = iptablesSync.Run(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

func (d *Daemon) startup(ctx context.Context) error {
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
	wireguard.CreateInterface(d.Iface)
	wireguard.SetIP(d.Iface, d.wormholeGateway)
	wireguard.SetPrivateKey(d.Iface, privKey)
	wireguard.SetListenPort(d.Iface, 9806) // TODO: (knisbet) make port configurable
	wireguard.SetUp(d.Iface)

	if d.OverlayCIDR == "" {
		// TODO (implement reading overlay cidr from network)
		return trace.NotImplemented("OverlayCIDR detection not implemented")
	}
	wireguard.SetRoute(d.Iface, d.OverlayCIDR)

	// configure CNI on the host
	err = d.configureCNI()
	if err != nil {
		return trace.Wrap(err)
	}

	d.startNodeController(ctx)
	err = d.startWireguardController(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

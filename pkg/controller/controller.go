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
	"net"
	"time"

	"github.com/gravitational/wormhole/pkg/wireguard"

	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

type Config struct {
	// NodeName is the name of the k8s node this instance is running on
	NodeName string

	// Namespace is the namespace that wormhole is running in for syncing state
	Namespace string

	// OverlayCIDR is the IP network CIDR for the entire overlay network
	OverlayCIDR string

	// NodeCIDR is the IP network CIDR for this particular node
	NodeCIDR string

	// Port is the external port wireguard should listen on for tunnels
	Port int

	// WireguardIface is the name of the wireguard interface to create
	WireguardIface string

	// BridgeIface is the name of the bridge to create
	BridgeIface string

	// Kubeconfig path is the path to a kubeconfig file for wormhole state exchange
	KubeconfigPath string

	// ResyncPeriod is how frequently to re-sync state between each component
	ResyncPeriod time.Duration
}

type Controller interface {
	Run(context.Context) error
}

type controller struct {
	logrus.FieldLogger

	config Config

	// client is a kubernetes client for accessing wormhole data
	client kubernetes.Interface

	// ipamInfo is IPAM info about the node
	ipamInfo *ipamInfo

	// wireguardInterface is a controller for managing / updating our wireguard interface
	wireguardInterface wireguard.Interface

	nodeController cache.Controller
	nodeLister     listers.NodeLister

	secretController cache.Controller
	secretLister     listers.SecretLister
}

func New(config Config) (Controller, error) {
	controller := &controller{
		config: config,
	}

	return controller, trace.Wrap(controller.init())
}

func (d *controller) init() error {
	d.Info("Initializing Wormhole...")

	var err error
	if d.config.KubeconfigPath != "" {
		d.client, err = getClientsetFromKubeconfig(d.config.KubeconfigPath)
		if err != nil {
			return trace.Wrap(err)
		}
	} else {
		clusterConfig, err := rest.InClusterConfig()
		if err != nil {
			return trace.Wrap(err)
		}
		d.client, err = kubernetes.NewForConfig(clusterConfig)
		if err != nil {
			return trace.Wrap(err)
		}
	}

	if d.config.NodeName == "" {
		d.Info("Attempting to detect Node Name.")
		err = d.detectNodeName()
		if err != nil {
			return trace.Wrap(err)
		}
	}

	if d.config.OverlayCIDR == "" {
		d.Info("Attempting to detect overlay network address range.")
		err = d.detectOverlayCidr()
		if err != nil {
			return trace.Wrap(err)
		}
	}

	if d.config.NodeCIDR == "" {
		d.Info("Attempting to detect node network address range")
		err = d.detectIPAM()
		if err != nil {
			return trace.Wrap(err)
		}
	}
	d.Info("Calculating IPAM Offsets.")
	err = d.calculateIPAMOffsets()
	if err != nil {
		return trace.Wrap(err)
	}

	if d.config.ResyncPeriod == 0 {
		d.config.ResyncPeriod = 60 * time.Second
	}

	d.Info("Initialization complete.")
	return nil
}

func (d *controller) Run(ctx context.Context) error {
	d.Info("Running wormhole controller.")
	d.Info("  Node Name:                   ", d.config.NodeName)
	d.Info("  Port:                        ", d.config.Port)
	d.Info("  Overlay Network:             ", d.config.OverlayCIDR)
	d.Info("  Node Network:                ", d.config.NodeCIDR)
	d.Info("  Wireguard Interface Name:    ", d.config.WireguardIface)
	d.Info("  Wireguard Interface Address: ", d.ipamInfo.wireguardAddr)
	d.Info("  Bridge Interface Name:       ", d.config.BridgeIface)
	d.Info("  Bridge Interface Address:    ", d.ipamInfo.bridgeAddr)
	d.Info("  Pod Address Start:           ", d.ipamInfo.podAddrStart)
	d.Info("  Pod Address End:             ", d.ipamInfo.podAddrEnd)
	d.Info("  Kubeconfig Path:             ", d.config.KubeconfigPath)
	d.Info("  Resync Period:               ", d.config.ResyncPeriod)

	_, overlayNetwork, err := net.ParseCIDR(d.config.OverlayCIDR)
	if err != nil {
		return trace.Wrap(err)
	}
	d.wireguardInterface, err = wireguard.New(wireguard.Config{
		InterfaceName: d.config.WireguardIface,
		IP:            d.ipamInfo.wireguardAddr,
		Port:          d.config.Port,
		OverlayNetworks: []net.IPNet{
			*overlayNetwork,
		},
	})
	if err != nil {
		return trace.Wrap(err)
	}

	err = d.configureCNI()
	if err != nil {
		return trace.Wrap(err)
	}

	// initialize the kubernetes secret object
	err = d.initKubeObjects()
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

/*
func (d *controller) startup(ctx context.Context) error {
	d.Info("Starting Wormhole...")
	var err error


	psk, err := d.getOrSetPSK()
	if err != nil {
		return trace.Wrap(err)
	}
	d.sharedKey = psk
	d.Info("PSK: <redacted>")

	pubKey, privKey, err := d.generateKeypair()
	if err != nil {
		return trace.Wrap(err)
	}
	d.publicKey = pubKey
	d.Info("PubKey: ", pubKey)
	d.Info("PrivKey: <redacted>")

	// send our new public key to the cluster
	err = d.publishPublicKey()
	if err != nil {
		return trace.Wrap(err)
	}

	// configure wireguard
	// TODO(knisbet) right now, I'm just ignoring errors in the setup
	// but this needs to eventually be handled properly
	_ = wireguard.CreateInterface(d.config.WireguardIface)
	_ = wireguard.SetIP(d.config.WireguardIface, d.wormholeGateway)
	_ = wireguard.SetPrivateKey(d.config.WireguardIface, privKey)
	_ = wireguard.SetListenPort(d.config.WireguardIface, d.config.Port)
	_ = wireguard.SetUp(d.config.WireguardIface)
	_ = wireguard.SetRoute(d.config.WireguardIface, d.config.OverlayCIDR)

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
*/

/*
func (d *controller) Run(ctx context.Context) error {
	d.Info("Starting wormhole Controller with config: ", spew.Sdump(d))

	iptablesSync := iptables.Config{
		FieldLogger:    d.FieldLogger.WithField("module", "iptables"),
		OverlayCIDR:    d.config.OverlayCIDR,
		PodCIDR:        d.nodePodCIDR,
		WireguardIface: d.config.WireguardIface,
		BridgeIface:    d.config.BridgeIface,
	}
	err := iptablesSync.Run(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}
*/

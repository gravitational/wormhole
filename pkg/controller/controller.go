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

	wormholeclientset "github.com/gravitational/wormhole/pkg/client/clientset/versioned"
	wormholelister "github.com/gravitational/wormhole/pkg/client/listers/wormhole.gravitational.io/v1beta1"
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

type Config struct {
	// NodeName is the name of the k8s node this instance is running on
	NodeName string

	// Namespace is the kubernetes namespace to use for syncing wormhole objects
	Namespace string

	// OverlayCIDR is the IP network in CIDR format for the entire overlay network
	OverlayCIDR string

	// NodeCIDR is the IP network in CIDR format assigned to this node
	NodeCIDR string

	// ListenPort is the external port wireguard should listen on between hosts
	ListenPort int

	// WireguardIface is the name of the wireguard interface for encrypted node to node traffic
	WireguardIface string

	// BridgeIface is the name of the linux bridge to create internal to the host
	BridgeIface string

	// Kubeconfig path is the path to a kubeconfig file to access the kubernetes API
	KubeconfigPath string

	// SyncInterval is how frequently to re-sync state between each component
	SyncInterval time.Duration

	// Endpoint is the networking address that is available for routing between all wireguard nodes
	// In general this should be the same as the AdvertiseIP Address of the node
	Endpoint string

	// BridgeMTU is the MTU value to assign to the internal linux bridge
	BridgeMTU int
}

type Controller interface {
	Run(context.Context) error
}

type controller struct {
	logrus.FieldLogger

	config Config

	// client is a kubernetes client for accessing wormhole data
	client    kubernetes.Interface
	crdClient wormholeclientset.Interface

	// ipamInfo is IPAM info about the node
	ipamInfo *ipamInfo

	//overlayCidr is the IP network for the entire overlay network
	overlayCIDR net.IPNet

	// wireguardInterface is a controller for managing / updating our wireguard interface
	wireguardInterface wireguard.Interface

	nodeController cache.Controller
	nodeLister     wormholelister.WgnodeLister

	secretController cache.Controller
	secretLister     listers.SecretLister

	resyncC chan interface{}
}

func New(config Config, logger logrus.FieldLogger) (Controller, error) {
	controller := &controller{
		FieldLogger: logger,
		config:      config,
		resyncC:     make(chan interface{}, 1),
	}

	return controller, trace.Wrap(controller.init())
}

func (d *controller) init() error {
	d.Info("Initializing Wormhole...")

	if d.config.BridgeMTU < 68 || d.config.BridgeMTU > 65535 {
		return trace.BadParameter("Bridge MTU value out of range. %v must be within 68-65535", d.config.BridgeMTU)
	}
	if d.config.BridgeMTU < 1280 {
		d.WithField("mtu", d.config.BridgeMTU).
			Warn("Bridge MTU is small, you may experience performance issues. 1280 or more is recommended")
	}

	var err error
	var config *rest.Config
	if d.config.KubeconfigPath != "" {
		config, err = clientcmd.BuildConfigFromFlags("", d.config.KubeconfigPath)
		if err != nil {
			return trace.Wrap(err)
		}
	} else {
		config, err = rest.InClusterConfig()
		if err != nil {
			return trace.Wrap(err)
		}
	}

	d.client, err = kubernetes.NewForConfig(config)
	if err != nil {
		return trace.Wrap(err)
	}

	d.crdClient, err = wormholeclientset.NewForConfig(config)
	if err != nil {
		return trace.Wrap(err)
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
		err = d.detectOverlayCIDR()
		if err != nil {
			return trace.Wrap(err)
		}
	}
	_, overlayNetwork, err := net.ParseCIDR(d.config.OverlayCIDR)
	if err != nil {
		return trace.Wrap(err)
	}
	d.overlayCIDR = *overlayNetwork

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

	if d.config.SyncInterval == 0 {
		d.config.SyncInterval = 60 * time.Second
	}

	d.Info("Initialization complete.")
	return nil
}

func (d *controller) Run(ctx context.Context) error {
	d.Info("Running wormhole controller.")
	d.Info("  Node Name:                   ", d.config.NodeName)
	d.Info("  Port:                        ", d.config.ListenPort)
	d.Info("  Overlay Network:             ", d.config.OverlayCIDR)
	d.Info("  Node Network:                ", d.config.NodeCIDR)
	d.Info("  Wireguard Interface Name:    ", d.config.WireguardIface)
	d.Info("  Wireguard Interface Address: ", d.ipamInfo.wireguardAddr)
	d.Info("  Bridge Interface Name:       ", d.config.BridgeIface)
	d.Info("  Bridge Interface Address:    ", d.ipamInfo.bridgeAddr)
	d.Info("  Bridge MTU:                  ", d.config.BridgeMTU)
	d.Info("  Pod Address Start:           ", d.ipamInfo.podAddrStart)
	d.Info("  Pod Address End:             ", d.ipamInfo.podAddrEnd)
	d.Info("  Kubeconfig Path:             ", d.config.KubeconfigPath)
	d.Info("  Resync Period:               ", d.config.SyncInterval)

	iptablesSync := iptables.Config{
		FieldLogger:    d.FieldLogger.WithField("module", "iptables"),
		OverlayCIDR:    d.config.OverlayCIDR,
		PodCIDR:        d.config.NodeCIDR,
		WireguardIface: d.config.WireguardIface,
		BridgeIface:    d.config.BridgeIface,
		SyncInterval:   d.config.SyncInterval,
	}
	err := iptablesSync.Run(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	d.wireguardInterface, err = wireguard.New(wireguard.Config{
		InterfaceName: d.config.WireguardIface,
		IP:            d.ipamInfo.wireguardAddr,
		ListenPort:    d.config.ListenPort,
		OverlayNetworks: []net.IPNet{
			d.overlayCIDR,
		},
	}, d.FieldLogger)
	if err != nil {
		return trace.Wrap(err)
	}

	err = d.configureCNI()
	if err != nil {
		return trace.Wrap(err)
	}

	err = d.publishNodeInfo()
	if err != nil {
		return trace.Wrap(err)
	}

	err = d.initKubeObjects()
	if err != nil {
		return trace.Wrap(err)
	}

	d.startNodeWatcher(ctx)
	d.startSecretWatcher(ctx)
	d.startNodeDeletionWatcher(ctx)

	err = d.waitForControllerSync(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	err = d.updatePeerSecrets(true)
	if err != nil {
		return trace.Wrap(err)
	}

	d.Info("Wormhole is running")
	return trace.Wrap(d.run(ctx))
}

func (d *controller) run(ctx context.Context) error {
	syncTimer := time.NewTicker(d.config.SyncInterval)
	defer syncTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			return trace.Wrap(ctx.Err())
		case <-d.resyncC:
			err := d.resync()
			if err != nil {
				return trace.Wrap(err)
			}
		case <-syncTimer.C:
			err := d.resync()
			if err != nil {
				return trace.Wrap(err)
			}
		}
	}
}

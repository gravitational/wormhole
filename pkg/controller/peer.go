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
	"fmt"
	"net"
	"strings"

	"github.com/gravitational/wormhole/pkg/wireguard"
	"github.com/sirupsen/logrus"

	"github.com/google/go-cmp/cmp"
	"github.com/gravitational/trace"
	"k8s.io/api/core/v1"
)

type Peer struct {
	PublicKey   string
	Address     net.UDPAddr
	AllowedCIDR []net.IPNet
}

// NodeToPeer converts a kubernetes Node into a Wireguard Peer
func NodeToPeer(node *v1.Node) (*Peer, error) {
	addr, err := getNodeAddr(node)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	publicKey, ok := node.Annotations[annotationWireguardPublicKey]
	if !ok {
		return nil, trace.BadParameter("Node %v unable to find annotation '%v' in annotations: %v",
			node.Name, annotationWireguardPublicKey, node.Annotations)
	}

	_, network, err := net.ParseCIDR(node.Spec.PodCIDR)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return &Peer{
		PublicKey:   publicKey,
		Address:     addr,
		AllowedCIDR: []net.IPNet{*network},
	}, nil
}

func PeerStatusToPeer(peerStatus wireguard.PeerStatus) (*Peer, error) {
	addr, err := net.ResolveUDPAddr("udp", peerStatus.Endpoint)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	var allowedCIDR []net.IPNet
	if peerStatus.AllowedIP != "" {
		split := strings.Split(peerStatus.AllowedIP, ",")
		for _, cidr := range split {
			_, network, err := net.ParseCIDR(cidr)
			if err != nil {
				return nil, trace.WrapWithMessage(err, "failed parsing network: %v", network)
			}
			allowedCIDR = append(allowedCIDR, *network)
		}
	}

	return &Peer{
		PublicKey:   peerStatus.PublicKey,
		Address:     *addr,
		AllowedCIDR: allowedCIDR,
	}, nil
}

// Equals compares two nodes for equality
func (n Peer) Equals(other Peer) bool {
	return cmp.Equal(n, other)
}

func (d *Controller) RemovePeer(peer Peer) error {
	return trace.Wrap(wireguard.RemovePeer(d.WireguardIface, peer.PublicKey))
}

func (d *Controller) AddPeer(peer Peer) error {

	return trace.Wrap(wireguard.AddPeer(
		d.WireguardIface, peer.PublicKey, d.sharedKey, peer.AllowedCIDRString(), peer.Address.String()))
}

func (n Peer) AllowedCIDRString() string {
	allowedIPs := make([]string, len(n.AllowedCIDR))
	for i, net := range n.AllowedCIDR {
		allowedIPs[i] = net.String()
	}
	return strings.Join(allowedIPs, ",")
}

func (n Peer) Fields(prefix string) logrus.Fields {
	return logrus.Fields{
		fmt.Sprint(prefix, "peer"):        n.PublicKey,
		fmt.Sprint(prefix, "address"):     n.Address.String(),
		fmt.Sprint(prefix, "allowed_ips"): n.AllowedCIDRString(),
	}
}

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

/*
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

// Equals compares two nodes for equality
func (n Peer) Equals(other Peer) bool {
	return cmp.Equal(n, other)
}

func (d *controller) RemovePeer(peer Peer) error {
	return trace.Wrap(wireguard.RemovePeer(d.config.WireguardIface, peer.PublicKey))
}

func (d *controller) AddPeer(peer Peer) error {

	return trace.Wrap(wireguard.AddPeer(
		d.config.WireguardIface, peer.PublicKey, d.sharedKey, peer.AllowedCIDRString(), peer.Address.String()))
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
*/

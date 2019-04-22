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

package wireguard

import (
	"net"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"

	"github.com/vishvananda/netlink"

	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"
)

type Config struct {
	// InterfaceName is the name of the wireguard interface to manage
	InterfaceName string
	// IP Address to assign to the interface
	IP string
	// External Port to have wireguard listen on
	ListenPort int
	// OverlayNetworks is the IP range(s) for the entire overlay network
	OverlayNetworks []net.IPNet
}

type Peer struct {
	// PublicKey is the wireguard public key of the peer
	PublicKey string
	// SharedKey is the shared secret between the local instance and the peer
	SharedKey string
	// AllowedIP is the list of CIDR's to accept between hosts
	AllowedIP []string
	// Endpoint is the IP:Port endpoint of the peer
	Endpoint string
}

type PeerStatus struct {
	// PublicKey is the wireguard public key of the peer
	PublicKey string
	// SharedKey is the shared secret between the local instance and the peer
	SharedKey string
	// Endpoint is the IP:Port endpoint of the peer
	Endpoint string
	// AllowedIP is a CSV of the allowed IP ranges of the peer
	AllowedIP string
	// LastHandshakeTime is the timestamp of the last handshake with the peer
	LastHandshake time.Time
	// BytesTX is the number of bytes transmitted to the peer
	BytesTX int64
	// BytesRX is the number of bytes received from the peer
	BytesRX int64
	//Keepalive is the number of second to keep the connection alive
	Keepalive int
}

type Interface interface {
	PublicKey() string
	SyncPeers(map[string]Peer) error
	GenerateSharedKey() (string, error)
}

type iface struct {
	logrus.FieldLogger
	Config

	publicKey string
	wg        Wg
}

func New(config Config, logger logrus.FieldLogger) (Interface, error) {
	err := config.CheckAndSetDefaults()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	wg := &wg{
		iface: config.InterfaceName,
	}

	iface, err := new(config, wg, logger)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// retrieve the wireguard netlink device
	link, err := netlink.LinkByName(config.InterfaceName)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// add overlay network routes towards the wireguard interface
	// TODO(knisbet) consider making this part of the control loop, so that if the system is changed for any reason
	// the routes will get re-created
	for _, network := range config.OverlayNetworks {
		dst := network
		err = netlink.RouteAdd(&netlink.Route{
			LinkIndex: link.Attrs().Index,
			Dst:       &dst,
		})
		if err != nil && !strings.Contains(err.Error(), "file exists") {
			return nil, trace.Wrap(err)
		}
	}

	return iface, nil

}

func new(config Config, wg Wg, logger logrus.FieldLogger) (*iface, error) {
	key, err := wg.genKey()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	pubKey, err := wg.pubKey(key)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	err = wg.deleteInterface()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	err = wg.createInterface()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	err = wg.setIP(config.IP)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	err = wg.setPrivateKey(key)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	err = wg.setListenPort(config.ListenPort)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	err = wg.setUp()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return &iface{
		FieldLogger: logger.WithField(trace.Component, "iface"),
		publicKey:   pubKey,
		wg:          wg,
	}, nil
}

func (c *Config) CheckAndSetDefaults() error {
	if c.InterfaceName == "" {
		return trace.BadParameter("Interface name is required")
	}
	if c.IP == "" {
		return trace.BadParameter("IP address is required")
	}
	if c.ListenPort == 0 {
		return trace.BadParameter("Port is required")
	}

	_, ipv4Net, err := net.ParseCIDR(c.IP)
	if err != nil {
		return trace.Wrap(err)
	}

	// only ipv4 is currently supported
	if ipv4Net.IP.To4() == nil {
		return trace.BadParameter("%v is not an ipv4 subnet", c.IP)
	}

	return nil
}

func (i iface) PublicKey() string {
	return i.publicKey
}

func (i iface) SyncPeers(peers map[string]Peer) error {
	i.Debug("Syncing peers to wireguard.")

	// get the peers that are locally configured within wireguard
	peerStatuses, err := i.wg.getPeers()
	if err != nil {
		i.Warn("Error reading peers from wireguard.")
		return trace.Wrap(err)
	}
	i.Debug("peer status: ", spew.Sdump(peerStatuses))

	// iterate through each peer, and find the corresponding desired peer
	// peer = local wireguard peer
	// desiredPeer = peer as per k8s API
	for _, peerStatus := range peerStatuses {
		desiredPeer, ok := peers[peerStatus.PublicKey]
		if ok {
			// if there is a difference in the peer, delete and re-add the peer
			if !peerStatus.ToPeer().Equals(desiredPeer) {
				log := i.WithField("peer", desiredPeer.PublicKey)
				log.Info("Re-creating peer.")

				err = i.wg.removePeer(peerStatus.PublicKey)
				if err != nil {
					return trace.Wrap(err)
				}

				err = i.wg.addPeer(desiredPeer)
				if err != nil {
					return trace.Wrap(err)
				}
			}
		} else {
			// peer doesn't exist in desired peers, so we should delete from wireguard

			log := i.WithField("peer", peerStatus.PublicKey)
			log.Infof("Deleting unexpected peer: %+v", peerStatus.ToPeer())

			err = i.wg.removePeer(peerStatus.PublicKey)
			if err != nil {
				return trace.Wrap(err)
			}
		}
	}

	// iterate over each desired peer, looking for a peer missing in wireguard
	i.Debug("Checking for missing peers")
	for _, desiredPeer := range peers {
		if _, ok := peerStatuses[desiredPeer.PublicKey]; !ok {
			err = i.wg.addPeer(desiredPeer)
			if err != nil {
				return trace.Wrap(err)
			}
		}
	}

	i.Debug("Syncing peers to wireguard complete.")
	return nil
}

func (i iface) GenerateSharedKey() (string, error) {
	return i.wg.genPSK()
}

func (i iface) AddPeer(peer Peer) error {
	log := i.WithField("peer", peer.PublicKey)
	log.Infof("Adding peer: %+v", peer)

	return trace.Wrap(i.wg.addPeer(peer))
}

func (i iface) RemovePeer(publicKey string) error {
	log := i.WithField("peer", publicKey)
	log.Info("Removing peer.")

	return trace.Wrap(i.wg.removePeer(publicKey))
}

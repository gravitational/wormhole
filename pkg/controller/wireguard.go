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
	"github.com/sirupsen/logrus"
)

type wireguardSync struct {
	logrus.FieldLogger
	*controller
}

func (d *wireguardSync) init() {
	d.FieldLogger = d.controller.FieldLogger.WithField("module", "wireguard-sync")
}

/*
// wireguard controller, monitors the local wireguard interface, and resyncs with the kubernetes cache
func (d *wireguardSync) start(ctx context.Context) error {
	d.init()

	err := d.syncWireguardWithK8s()
	if err != nil {
		return trace.Wrap(err)
	}

	d.Info("Now monitoring wireguard for configuration differences.")
	go d.run(ctx)

	return nil
}

func (d *wireguardSync) run(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := d.syncWireguardWithK8s()
			if err != nil {
				d.WithField("module", "wireguard-monitor").Warn("Error syncing wireguard with kubernetes: ", err)
			}
		}
	}
}
*/
/*
func (d *wireguardSync) syncWireguardWithK8s() error {
	// get the peers that are locally configured within wireguard
	peerStatuses, err := wireguard.GetPeerStatus(d.config.WireguardIface)
	if err != nil {
		return trace.Wrap(err)
	}

	// get the peers from k8s, and convert to a map indexed by public key
	nodes, err := d.controller.nodeList.List(labels.Everything())
	if err != nil {
		return trace.Wrap(err)
	}
	desiredPeers := make(map[string]Peer)
	for _, node := range nodes {
		// skip ourselves
		if node.Name == d.config.NodeName {
			continue
		}

		peer, err := NodeToPeer(node)
		if err != nil {
			d.WithField("node", node.Name).Info("unable to convert v1.Node to Peer: ", trace.DebugReport(err))
			continue
		}

		desiredPeers[peer.PublicKey] = *peer
	}

	// iterate through each peer, and find the corresponding desired peer
	// peer = local wireguard peer
	// desiredPeer = peer as per k8s API
	for _, peerStatus := range peerStatuses {
		l := d.WithField("peer", peerStatus.PublicKey)

		desiredPeer, ok := desiredPeers[peerStatus.PublicKey]
		if ok {
			peer, err := PeerStatusToPeer(peerStatus)
			if err != nil || !desiredPeer.Equals(*peer) {
				// I'm not sure if this is the best approach, but for now, if we have any issue converting the peer
				// just delete and re-add the peer
				l.Info("re-creating peer: ", trace.DebugReport(err))

				err = d.RemovePeer(desiredPeer)
				if err != nil {
					l.WithField("peer", desiredPeer.PublicKey).Info("Error removing peer: ", trace.DebugReport(err))
				}
				err = d.AddPeer(desiredPeer)
				if err != nil {
					l.WithField("peer", desiredPeer.PublicKey).Info("Error adding peer: ", trace.DebugReport(err))
				}
				continue
			}
		} else {
			// peer doesn't exist, we should delete it
			l.Info("Deleting wireguard peer that isn't in desired state")
			err = wireguard.RemovePeer(d.config.WireguardIface, peerStatus.PublicKey)
			if err != nil {
				l.WithField("peer", desiredPeer.PublicKey).Info("Error removing peer: ", trace.DebugReport(err))
			}
		}
	}

	// iterate over each node, looking for a missing peer
	for _, desiredPeer := range desiredPeers {
		if _, ok := peerStatuses[desiredPeer.PublicKey]; !ok {
			d.WithField("peer", desiredPeer.PublicKey).Info("Adding missing wireguard peer.")
			err = d.AddPeer(desiredPeer)
			if err != nil {
				d.WithField("peer", desiredPeer.PublicKey).Info("Error adding peer: ", trace.DebugReport(err))
			}
		}
	}

	d.Info("Wireguard re-sync complete.")

	return nil
}
*/

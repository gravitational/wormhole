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
	"fmt"
	"time"

	"github.com/gravitational/wormhole/pkg/wireguard"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/gravitational/trace"
)

// wireguard controller, monitors the local wireguard interface, and resyncs with the kubernetes cache
func (d *Daemon) startWireguardController(ctx context.Context) error {
	// wait for the kubernetes controllers to be in sync
	l := d.WithField("module", "wireguard-monitor")
	l.Info("Waiting for kubernetes API sync to complete")
	err := d.waitForNodeControllerSync(ctx)
	if err != nil {
		return trace.Wrap(err)
	}
	l.Info("Kubernetes API sync completed")
	err = d.syncWireguardWithK8s()
	if err != nil {
		return trace.Wrap(err)
	}

	go d.runWireguardController(ctx)

	return nil
}

func (d *Daemon) runWireguardController(ctx context.Context) {
	l := d.WithField("module", "wireguard-monitor")
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := d.syncWireguardWithK8s()
			if err != nil {
				l.Warn("Error syncing wireguard with kubernetes: ", err)
			}
		}
	}
}

func (d *Daemon) syncWireguardWithK8s() error {
	l := d.WithField("module", "wireguard-monitor")
	peers, err := wireguard.GetPeerStatus(d.Iface)
	if err != nil {
		return trace.Wrap(err)
	}

	// convert our peers list to a map indexed by public key
	peerMap := make(map[string]wireguard.PeerStatus)
	for _, peer := range peers {
		peerMap[peer.PublicKey] = peer
	}

	nodes, err := d.nodeList.List(labels.Everything())
	if err != nil {
		return trace.Wrap(err)
	}

	nodeMap := make(map[string]*v1.Node)
	for _, node := range nodes {
		if node.Name == d.NodeName {
			continue
		}
		publicKey := getNodePublicKey(node)
		addr := getNodeIP(node)
		cidr := node.Spec.PodCIDR

		if publicKey != "" && addr != "" && cidr != "" {
			nodeMap[publicKey] = node
		}

	}

	var errors []error
	// iterate through each peer, and find the corresponding node object
	for _, peer := range peers {
		node, ok := nodeMap[peer.PublicKey]
		if ok {
			publicKey := getNodePublicKey(node)
			addr := getNodeIP(node)
			cidr := node.Spec.PodCIDR

			if cidr != peer.AllowedIP || fmt.Sprint(addr, ":9806") != peer.Endpoint {
				//mismatched endpoint/ try deleting and readding the peer
				// TODO(knisbet) SECURITY: this may be where we need to flag a changed peer endpoint for a stolen
				// private key. But this behaviour needs to be verified. For now, the naive implementation is to
				// delete and re-add the peer
				l.Warn("Peer mismatch detected. Updating peer to match kubernetes node:")
				l.Warnf("  cidr: %v -> %v", peer.AllowedIP, cidr)
				l.Warnf("  addr: %v -> %v", peer.Endpoint, addr)
				err = wireguard.RemovePeer(d.Iface, peer.PublicKey)
				if err != nil {
					errors = append(errors, err)
				}
				// TODO(knisbet) remove port hard code
				err = wireguard.AddPeer(d.Iface, publicKey, d.sharedKey, cidr, addr, "9806")
				if err != nil {
					errors = append(errors, err)
				}
			}
		} else {
			// peer doesn't exist, we should delete it
			l.WithField("public_key", peer.PublicKey).Warn("Removing wireguard peer that doesn't match any k8s node.")
			err = wireguard.RemovePeer(d.Iface, peer.PublicKey)
			if err != nil {
				errors = append(errors, err)
			}
		}
	}

	// iterate over each node, looking for a missing peer
	for _, node := range nodeMap {
		publicKey := getNodePublicKey(node)
		addr := getNodeIP(node)
		cidr := node.Spec.PodCIDR

		if _, ok := peerMap[publicKey]; !ok {
			l.WithField("public_key", publicKey).Warn("Adding missing wireguard peer.")
			// TODO(knisbet) remove port hard code
			err = wireguard.AddPeer(d.Iface, publicKey, d.sharedKey, cidr, addr, "9806")
			if err != nil {
				errors = append(errors, err)
			}
		}
	}

	l.Info("Wireguard re-sync complete.")
	return trace.NewAggregate(errors...)
}

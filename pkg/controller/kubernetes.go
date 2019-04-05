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
	"fmt"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/sirupsen/logrus"

	"github.com/gravitational/trace"
	"github.com/gravitational/wormhole/pkg/apis/wormhole.gravitational.io/v1beta1"
	wormholelister "github.com/gravitational/wormhole/pkg/client/listers/wormhole.gravitational.io/v1beta1"
	"github.com/gravitational/wormhole/pkg/wireguard"

	"github.com/cenkalti/backoff"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	secretObjectName = "wireguard-shared-secrets"
)

// initKubeObjects runs during startup and is meant to try and create empty kubernetes objects that wormhole uses to
// exchange state via the kubernetes API.
// This just tries to create the object, and ignores already exists errors.
func (c *controller) initKubeObjects() error {
	c.Debugf("Initializing secret %v/%v", c.config.Namespace, secretObjectName)

	_, err := c.client.CoreV1().Secrets(c.config.Namespace).Create(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: secretObjectName,
		},
	})

	if errors.IsAlreadyExists(err) {
		return nil
	}

	return trace.Wrap(err)

}

// publishNodeInfo publishes the node configuration to the kubernetes API for the rest of the cluster to pick up
func (c *controller) publishNodeInfo() error {
	node := &v1beta1.Wgnode{
		ObjectMeta: metav1.ObjectMeta{
			Name: c.config.NodeName,
		},
		Status: v1beta1.WgnodeStatus{
			Port:      c.config.ListenPort,
			PublicKey: c.wireguardInterface.PublicKey(),
			NodeCIDR:  c.config.NodeCIDR,
			Endpoint:  c.config.Endpoint,
		},
	}
	c.Debug("Publishing Node Information to kubernetes: ", spew.Sdump(node))
	_, err := c.crdClient.WormholeV1beta1().Wgnodes(c.config.Namespace).Create(node)
	if err == nil {
		return nil
	}
	if errors.IsAlreadyExists(err) {
		node, err = c.crdClient.WormholeV1beta1().Wgnodes(c.config.Namespace).Get(c.config.NodeName, metav1.GetOptions{})
		if err != nil {
			return trace.Wrap(err)
		}

		node.Status.Port = c.config.ListenPort
		node.Status.PublicKey = c.wireguardInterface.PublicKey()
		node.Status.NodeCIDR = c.config.NodeCIDR
		node.Status.Endpoint = c.config.Endpoint

		_, err = c.crdClient.WormholeV1beta1().Wgnodes(c.config.Namespace).Update(node)
	}
	return trace.Wrap(err)
}

func (c *controller) startNodeWatcher(ctx context.Context) {
	indexer, controller := cache.NewIndexerInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return c.crdClient.WormholeV1beta1().Wgnodes(c.config.Namespace).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return c.crdClient.WormholeV1beta1().Wgnodes(c.config.Namespace).Watch(options)
			},
		}, &v1beta1.Wgnode{},
		0,
		c.handlerFuncs(),
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)

	c.nodeController = controller
	c.nodeLister = wormholelister.NewWgnodeLister(indexer)

	go c.runNodeWatcher(ctx)
}

func (c *controller) runNodeWatcher(ctx context.Context) {
	stopCh := make(chan struct{})
	defer close(stopCh)

	go c.nodeController.Run(stopCh)

	<-ctx.Done()
}

func (c *controller) handlerFuncs() cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    c.handleAdded,
		UpdateFunc: c.handleUpdated,
		DeleteFunc: c.handleAdded,
	}
}

func (c *controller) handleAdded(obj interface{}) {
	select {
	case c.resyncC <- nil:
		// trigger resync
	default:
		// don't block
	}
}
func (c *controller) handleUpdated(oldObj interface{}, newObj interface{}) {
	select {
	case c.resyncC <- nil:
		// trigger resync
	default:
		// don't block
	}
}

func (c *controller) startSecretWatcher(ctx context.Context) {
	indexer, controller := cache.NewIndexerInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return c.client.CoreV1().Secrets(c.config.Namespace).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return c.client.CoreV1().Secrets(c.config.Namespace).Watch(options)
			},
		}, &v1.Secret{},
		0,
		c.handlerFuncs(),
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)

	c.secretController = controller
	c.secretLister = listers.NewSecretLister(indexer)

	go c.runSecretWatcher(ctx)
}

func (c *controller) runSecretWatcher(ctx context.Context) {
	stopCh := make(chan struct{})
	defer close(stopCh)

	go c.secretController.Run(stopCh)

	<-ctx.Done()
}

func (c *controller) waitForControllerSync(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if c.nodeController.HasSynced() && c.secretController.HasSynced() {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}

}

func (c *controller) resync() error {
	c.Debug("Re-sync triggered")

	err := backoff.Retry(func() error {
		err := c.syncWithWireguard()
		return trace.Wrap(err)
	}, &backoff.ExponentialBackOff{
		InitialInterval:     500 * time.Millisecond,
		MaxInterval:         5 * time.Second,
		RandomizationFactor: 0.1,
		Multiplier:          2,
		MaxElapsedTime:      10 * time.Second,
		Clock:               backoff.SystemClock,
	})
	if err != nil {
		return trace.Wrap(err)
	}

	return trace.Wrap(c.updatePeerSecrets(false))
}

func (c *controller) syncWithWireguard() error {
	c.Debug("Re-syncing wireguard configuration")
	defer c.Debug("Re-sync wireguard complete")

	nodes, err := c.nodeLister.List(labels.NewSelector())
	if err != nil {
		return trace.Wrap(err)
	}

	c.Debugf("Get secret %v/%v", c.config.Namespace, secretObjectName)
	sharedSecrets, err := c.secretLister.Secrets(c.config.Namespace).Get(secretObjectName)
	if err != nil {
		return trace.Wrap(err)
	}

	peers := make(map[string]wireguard.Peer, len(nodes))
	for _, node := range nodes {
		// we don't connect to ourselves
		if node.Name == c.config.NodeName {
			continue
		}

		ss, ok := sharedSecrets.Data[nodePairKey(c.config.NodeName, node.Name)]
		if !ok {
			// Skip if a shared secret doesn't currently exist for this peer
			// it will get created later
			continue
		}

		endpoint := fmt.Sprintf("%v:%v", node.Status.Endpoint, node.Status.Port)
		peers[node.Status.PublicKey] = wireguard.Peer{
			PublicKey: node.Status.PublicKey,
			SharedKey: string(ss),
			AllowedIP: []string{node.Status.NodeCIDR},
			Endpoint:  endpoint,
		}
		c.WithFields(logrus.Fields{
			"public_key": node.Status.PublicKey,
			"allowed_ip": node.Status.NodeCIDR,
			"endpoint":   endpoint,
		}).Info("peer entry")
	}

	c.Debug("Sync with wireguard interface: ", spew.Sdump(peers))
	return trace.Wrap(c.wireguardInterface.SyncPeers(peers))
}

func nodePairKey(node1, node2 string) string {
	if strings.Compare(node1, node2) < 0 {
		return fmt.Sprintf("shared-secret-%v-%v", node1, node2)
	}
	return fmt.Sprintf("shared-secret-%v-%v", node2, node1)
}

// updatePeerSecrets generates new shared secrets for each peer
// overwrite - indicates whether to replace each shared key with a new one (used during startup to rotate each secret)
func (c *controller) updatePeerSecrets(overwrite bool) error {
	c.WithField("overwrite", overwrite).Debug("Updating peer shared secrets")
	nodes, err := c.nodeLister.List(labels.NewSelector())
	if err != nil {
		return trace.Wrap(err)
	}

	secretObject, err := c.secretLister.Secrets(c.config.Namespace).Get(secretObjectName)
	if err != nil {
		return trace.Wrap(err)
	}

	for _, node := range nodes {
		if node.Name == c.config.NodeName {
			continue
		}
		if secretObject.Data == nil {
			secretObject.Data = make(map[string][]byte)
		}

		// Only overwrite the secret if it doesn't already exist
		if _, ok := secretObject.Data[nodePairKey(c.config.NodeName, node.Name)]; !overwrite && ok {
			continue
		}

		psk, err := c.wireguardInterface.GenerateSharedKey()
		if err != nil {
			return trace.Wrap(err)
		}

		secretObject.Data[nodePairKey(c.config.NodeName, node.Name)] = []byte(psk)
	}

	// the peer secrets are based on last writer wins, so we ignore the object here
	// and it will be processed when our watchers get the updated object
	_, err = c.client.CoreV1().Secrets(c.config.Namespace).Update(secretObject)
	return trace.Wrap(err)
}

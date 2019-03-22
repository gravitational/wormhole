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

	"github.com/cenkalti/backoff"

	"github.com/gravitational/wormhole/pkg/wireguard"

	"github.com/gravitational/trace"
	"github.com/gravitational/wormhole/pkg/apis/wormhole.gravitational.io/v1beta1"
	wormholelister "github.com/gravitational/wormhole/pkg/client/listers/wormhole.gravitational.io/v1beta1"
	"k8s.io/api/core/v1"
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
	node := &v1beta1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: c.config.NodeName,
		},
		Status: v1beta1.NodeStatus{
			Port:      c.config.Port,
			PublicKey: c.wireguardInterface.PublicKey(),
			NodeCIDR:  c.config.NodeCIDR,
			Endpoint:  c.config.Endpoint,
		},
	}

	c.Debug("Attempting to publish node information object to kubernetes cluster (update)")
	_, err := c.crdClient.WormholeV1beta1().Nodes().UpdateStatus(node)
	if errors.IsNotFound(err) {
		c.Debug("Attempting to publish node information object to kubernetes cluster (create)")
		_, err = c.crdClient.WormholeV1beta1().Nodes().Create(node)
	}
	return trace.Wrap(err)
}

func (c *controller) startNodeWatcher(ctx context.Context) {
	indexer, controller := cache.NewIndexerInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return c.crdClient.WormholeV1beta1().Nodes().List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return c.crdClient.WormholeV1beta1().Nodes().Watch(options)
			},
		}, &v1beta1.Node{},
		60*time.Second,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.handleNodeAdded,
			UpdateFunc: c.handleNodeUpdated,
			DeleteFunc: c.handleNodeDeleted,
		},
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)

	c.nodeController = controller
	c.nodeLister = wormholelister.NewNodeLister(indexer)

	go c.runNodeWatcher(ctx)
}

func (c *controller) runNodeWatcher(ctx context.Context) {
	stopCh := make(chan struct{})
	defer close(stopCh)

	go c.nodeController.Run(stopCh)

	<-ctx.Done()
}

// handleNodeAdded
// Note: doesn't need to do anything, since adding a node also requires updating secrets
func (c *controller) handleNodeAdded(obj interface{}) {}
func (c *controller) handleNodeDeleted(obj interface{}) {
	err := c.syncWithWireguard()
	if err != nil {
		c.Warn("Erroring syncing with wireguard: ", trace.DebugReport(err))
	}
}
func (c *controller) handleNodeUpdated(oldObj interface{}, newObj interface{}) {
	err := c.syncWithWireguard()
	if err != nil {
		c.Warn("Erroring syncing with wireguard: ", trace.DebugReport(err))
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
		60*time.Second,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.handleSecretAdded,
			UpdateFunc: c.handleSecretUpdated,
			DeleteFunc: c.handleSecretDeleted,
		},
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

func (c *controller) handleSecretAdded(obj interface{}) {}
func (c *controller) handleSecretDeleted(obj interface{}) {
	err := c.syncWithWireguard()
	if err != nil {
		c.Warn("Erroring syncing with wireguard: ", trace.DebugReport(err))
	}
}
func (c *controller) handleSecretUpdated(oldObj interface{}, newObj interface{}) {
	err := c.syncWithWireguard()
	if err != nil {
		c.Warn("Erroring syncing with wireguard: ", trace.DebugReport(err))
	}
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

func (c *controller) syncWithWireguard() error {
	nodes, err := c.nodeLister.List(labels.NewSelector())
	if err != nil {
		return trace.Wrap(err)
	}

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
			// A shared secret doesn't exist between us and the peer node
			// This should be relatively rare, so trigger the handling of this in a separate routine
			go c.handleMissingPeerSharedSecret(node.Name)
			continue
		}

		peers[node.Status.PublicKey] = wireguard.Peer{
			PublicKey: node.Status.PublicKey,
			SharedKey: string(ss),
			AllowedIP: []string{node.Status.NodeCIDR},
			Endpoint:  node.Status.Endpoint,
		}
	}
	c.wireguardInterface.SyncPeers(peers)

	return nil
}

func nodePairKey(node1, node2 string) string {
	if strings.Compare(node1, node2) < 0 {
		return fmt.Sprintf("shared-secret-%v-%v", node1, node2)
	}
	return fmt.Sprintf("shared-secret-%v-%v", node2, node1)
}

// updatePeerSecrets goes through each known peer, and refreshes the shared secret used between the peers
// this ensures that all secrets are refreshed/rotated when processes are restarted.
func (c *controller) updatePeerSecrets() error {
	nodes, err := c.nodeLister.List(labels.NewSelector())
	if err != nil {
		return trace.Wrap(err)
	}

	secretObject, err := c.secretLister.Secrets(c.config.Namespace).Get(secretObjectName)
	if err != nil {
		return trace.Wrap(err)
	}

	for _, node := range nodes {
		psk, err := c.wireguardInterface.GenerateSharedKey()
		if err != nil {
			return trace.Wrap(err)
		}

		if secretObject.Data == nil {
			secretObject.Data = make(map[string][]byte)
		}
		secretObject.Data[nodePairKey(c.config.NodeName, node.Name)] = []byte(psk)
	}

	// the peer secrets are based on last writer wins, so we ignore the object here
	// and it will be processed when our watchers get the updated object
	_, err = c.client.CoreV1().Secrets(c.config.Namespace).Update(secretObject)
	return trace.Wrap(err)
}

func (c *controller) retryGeneratePeerSharedSecret(peer string) error {
	err := backoff.Retry(func() error {
		_, err := c.generatePeerSharedSecret(peer)
		return trace.Wrap(err)
	}, &backoff.ExponentialBackOff{
		InitialInterval:     1 * time.Second,
		RandomizationFactor: 0.2,
		Multiplier:          2,
		MaxElapsedTime:      30 * time.Second,
		Clock:               backoff.SystemClock,
	})
	return trace.Wrap(err)
}

func (c *controller) handleMissingPeerSharedSecret(peer string) {
	_, err := c.generatePeerSharedSecret(peer)
	if err != nil {
		c.Warn("failed to create shared secret for missing peer")
	}
}

// generatePeerSharedSecret generates a new wireguard pre-shared key, and write it to the kubernetes secret object
// and the secret is returned to the caller
func (c *controller) generatePeerSharedSecret(peer string) (string, error) {

	//secretObject, err := c.secretLister.Secrets(c.config.Namespace).Get(secretObjectName)
	secretObject, err := c.client.CoreV1().Secrets(c.config.Namespace).Get(secretObjectName, metav1.GetOptions{})
	if err != nil {
		return "", trace.Wrap(err)
	}

	psk, err := c.wireguardInterface.GenerateSharedKey()
	if err != nil {
		return "", trace.Wrap(err)
	}

	if secretObject.Data == nil {
		secretObject.Data = make(map[string][]byte)
	}
	secretObject.Data[nodePairKey(c.config.NodeName, peer)] = []byte(psk)

	_, err = c.client.CoreV1().Secrets(c.config.Namespace).Update(secretObject)
	return psk, trace.Wrap(err)

}

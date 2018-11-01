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
	"time"

	"github.com/gravitational/trace"
	"github.com/gravitational/wormhole/pkg/wireguard"
	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

type kubernetesSync struct {
	logrus.FieldLogger
	*Controller
}

func (d *kubernetesSync) init() {
	d.FieldLogger = d.Controller.FieldLogger.WithField("module", "k8s-sync")
}

func (d *kubernetesSync) start(ctx context.Context) {
	d.init()

	indexer, controller := cache.NewIndexerInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return d.wormholeClient.CoreV1().Nodes().List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return d.wormholeClient.CoreV1().Nodes().Watch(options)
			},
		}, &v1.Node{},
		120*time.Second,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    d.handleNodeAdded,
			UpdateFunc: d.handleNodeUpdated,
			DeleteFunc: d.handleNodeDeleted,
		},
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)

	d.Controller.nodeController = controller
	d.Controller.nodeList = listers.NewNodeLister(indexer)

	go d.run(ctx)

	// block startup until the controller has synced or the ctx has been cancelled
	_ = d.waitForNodeControllerSync(ctx)
}

func (d *kubernetesSync) run(ctx context.Context) {
	stopCh := make(chan struct{})
	defer close(stopCh)

	go d.Controller.nodeController.Run(stopCh)

	<-ctx.Done()
}

func (d *kubernetesSync) handleNodeAdded(obj interface{}) {
	node, ok := obj.(*v1.Node)
	if !ok {
		d.Warnf("handleNodeAdded received unexpected object: %T", obj)
		return
	}

	// ignore our own node
	if node.Name == d.NodeName {
		return
	}

	peer, err := NodeToPeer(node)
	if err != nil {
		d.WithField("node", node.Name).Info("Error converting new node to peer: ", err)
		return
	}

	d.WithFields(peer.Fields("")).WithField("node", node.Name).Info("Received new node from kubernetes.")

	err = d.AddPeer(*peer)
	if err != nil {
		d.WithFields(peer.Fields("")).WithField("node", node.Name).Error("Error adding peer: ", trace.DebugReport(err))
	}
}

func (d *kubernetesSync) handleNodeUpdated(oldObj interface{}, newObj interface{}) {
	oldNode, oOk := oldObj.(*v1.Node)
	newNode, nOk := newObj.(*v1.Node)
	if !oOk || !nOk {
		d.Warnf("handleNodeUpdated received unexpected object old: %T new: %T", oldObj, newObj)
		return
	}

	// ignore our own node
	if newNode.Name == d.NodeName {
		return
	}

	// ignore errors on converting the nodes
	oldPeer, _ := NodeToPeer(oldNode)
	newPeer, err := NodeToPeer(newNode)
	// Handle the new node being invalid
	if err != nil {
		d.WithField("node", newNode.Name).Info("Node updated with invalid configuration: ", err)

		// if the previous configuration was valid, delete the peer from wireguard
		if oldPeer != nil {
			d.WithFields(oldPeer.Fields("")).WithField("node", oldNode.Name).Info("Removing peer")
			err = d.RemovePeer(*oldPeer)
			if err != nil {
				d.WithFields(oldPeer.Fields("")).WithField("node", oldNode.Name).Warn("Error removing peer: ",
					trace.DebugReport(err))
			}
		}
		return
	}

	// if the previous configuration was invalid, but the new configuration is valid, add the peer
	if oldPeer == nil && newPeer != nil {
		d.WithFields(newPeer.Fields("")).WithField("node", newNode.Name).Info("Adding peer")
		err = d.AddPeer(*newPeer)
		if err != nil {
			d.WithFields(newPeer.Fields("")).WithField("node", newNode.Name).Warn("Error removing peer: ",
				trace.DebugReport(err))

		}
		return
	}

	// if we had a previously valid peer, and it's not equal to the new peer
	// we should update the peer
	if oldPeer != nil && !oldPeer.Equals(*newPeer) {
		d.WithFields(oldPeer.Fields("old_")).WithFields(newPeer.Fields("new_")).WithField("node", newNode.Name).
			Info("Replacing peer")

		err = d.RemovePeer(*oldPeer)
		if err != nil {
			d.WithFields(oldPeer.Fields("")).WithField("node", newNode.Name).Warn("Error removing peer: ",
				trace.DebugReport(err))
		}

		err = d.AddPeer(*newPeer)
		if err != nil {
			d.WithFields(newPeer.Fields("")).WithField("node", newNode.Name).Error("Error adding peer: ",
				trace.DebugReport(err))
		}

	}

}

func (d *kubernetesSync) handleNodeDeleted(obj interface{}) {
	node, ok := obj.(*v1.Node)
	if !ok {
		d.Warnf("handleNodeDeleted received unexpected object: %T", obj)
		return
	}

	l := d.WithField("node", node.Name)

	// ignore ok, just use "" as key if the annotation is missing
	publicKey := node.Annotations[annotationWireguardPublicKey]
	if publicKey == "" {
		l.Infof("Unknown peer %v/%v", node.Name, publicKey)
		return
	}

	l.Infof("Removing peer %v/%v", node.Name, publicKey)

	err := wireguard.RemovePeer(d.WireguardIface, publicKey)
	if err != nil {
		l.Warn("Error removing peer: ", trace.DebugReport(err))
	}
}

func (d *kubernetesSync) waitForNodeControllerSync(ctx context.Context) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if d.Controller.nodeController.HasSynced() {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}

}

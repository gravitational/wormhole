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
	"net"
	"os"
	"strconv"
	"time"

	"github.com/gravitational/trace"
	"github.com/gravitational/wormhole/pkg/wireguard"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	annotationWireguardPublicKey = "wireguard-public-key"
	annotationWireguardPort      = "wireguard-port"
)

func (d *Daemon) publishPublicKey() error {
	node, err := d.nodeClient.CoreV1().Nodes().Get(d.NodeName, metav1.GetOptions{})
	if err != nil {
		return trace.Wrap(err)
	}
	op := "add"
	if _, ok := node.Annotations[annotationWireguardPublicKey]; ok {
		op = "replace"
	}

	// technically this isn't race free
	// if another client adds/removes this annotation between the read/patch it could
	// generate a patching error.
	// but it's acceptable for now
	patch := fmt.Sprint(
		`[{"op": "`,
		op,
		`", "path": "/metadata/annotations/wireguard-public", "value":"`,
		d.publicKey,
		`"}]`,
	)

	d.WithField("node", d.NodeName).Info("Publishing public key: ", patch)
	_, err = d.nodeClient.CoreV1().Nodes().Patch(d.NodeName, types.JSONPatchType, []byte(patch))
	if err != nil {
		return trace.Wrap(err)
	}

	d.Debug("Publishing public key complete")

	return nil
}

func (d *Daemon) detectNodeName() error {
	d.Debug("Attempting to detect nodename.")
	defer func() { d.Info("Detected hostname: ", d.NodeName) }()
	// if we're running inside a pod
	// find the node the pod is assigned to
	podName := os.Getenv("POD_NAME")
	podNamespace := os.Getenv("POD_NAMESPACE")
	if podName != "" && podNamespace != "" {
		return trace.Wrap(d.updateNodeNameFromPod(podName, podNamespace))
	}

	nodeName, err := os.Hostname()
	if err != nil {
		return trace.Wrap(err)
	}
	// TODO(knisbet) we should probably validate here, a node object exists that matches our node name
	d.NodeName = nodeName
	return nil
}

func (d *Daemon) updateNodeNameFromPod(podName, podNamespace string) error {
	pod, err := d.nodeClient.CoreV1().Pods(podNamespace).Get(podName, metav1.GetOptions{})
	if err != nil {
		return trace.Wrap(err)
	}
	d.NodeName = pod.Spec.NodeName
	if d.NodeName == "" {
		return trace.BadParameter("node name not present in pod spec %v/%v", podNamespace, podName)
	}
	return nil
}

func (d *Daemon) getPodCIDR() error {
	d.Debug("Attempting to retrieve pod CIDR from k8s IPAM.")
	node, err := d.nodeClient.CoreV1().Nodes().Get(d.NodeName, metav1.GetOptions{})
	if err != nil {
		return trace.Wrap(err)
	}

	if node.Spec.PodCIDR == "" {
		return trace.BadParameter("node/%v node.spec.podCidr is missing", d.NodeName)
	}
	d.nodePodCIDR = node.Spec.PodCIDR
	d.Info("PodCIDR: ", d.nodePodCIDR)

	_, ipv4Net, err := net.ParseCIDR(d.nodePodCIDR)
	if err != nil {
		return trace.Wrap(err)
	}

	bits, _ := ipv4Net.Mask.Size()
	if bits < 24 {
		return trace.BadParameter("podCIDR needs to be at least 24 bits. %v is only %v bits.", d.nodePodCIDR, bits)
	}

	if ipv4Net.IP.To4() == nil {
		return trace.BadParameter("%v is not an ipv4 subnet", d.nodePodCIDR)
	}

	gateway := net.IP(append([]byte(nil), ipv4Net.IP.To4()...))
	gateway[3]++

	wormhole := net.IPNet{
		IP:   net.IP(append([]byte(nil), ipv4Net.IP.To4()...)),
		Mask: []byte{255, 255, 255, 255}, // /32
	}
	wormhole.IP[3] += 2

	rangeStart := net.IP(append([]byte(nil), ipv4Net.IP.To4()...))
	rangeStart[3] += 10

	rangeEnd := net.IP(append([]byte(nil), ipv4Net.IP.To4()...))
	rangeEnd[3] += 210

	d.podGateway = gateway.String()
	d.wormholeGateway = wormhole.String()
	d.podRangeStart = rangeStart.String()
	d.podRangeEnd = rangeEnd.String()

	d.Info("PodGateway: ", d.podGateway)
	d.Info("WormholeGateway: ", d.wormholeGateway)
	d.Info("PodRangeStart: ", d.podRangeStart)
	d.Info("PodRangeEnd: ", d.podRangeEnd)

	return nil
}

func (d *Daemon) startNodeController(ctx context.Context) {

	indexer, controller := cache.NewIndexerInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return d.wormholeClient.CoreV1().Nodes().List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return d.wormholeClient.CoreV1().Nodes().Watch(options)
			},
		}, &v1.Node{},
		15*time.Second, // TODO(knisbet) this is set way to low for testing purposes
		cache.ResourceEventHandlerFuncs{
			AddFunc:    d.handleNodeAdded,
			UpdateFunc: d.handleNodeUpdated,
			DeleteFunc: func(obj interface{}) {

			},
		},
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)

	d.nodeController = controller
	d.nodeList = listers.NewNodeLister(indexer)

	go d.runNodeController(ctx)
}

func (d *Daemon) runNodeController(ctx context.Context) {
	stopCh := make(chan struct{})
	defer close(stopCh)

	go d.nodeController.Run(stopCh)

	<-ctx.Done()
}

func (d *Daemon) handleNodeAdded(obj interface{}) {
	node, ok := obj.(*v1.Node)
	if !ok {
		d.Warnf("handleNodeAdded received unexpected object: %T", obj)
		return
	}

	publicKey := getNodePublicKey(node)
	addr := getNodeIP(node)
	cidr := node.Spec.PodCIDR

	l := d.WithField("node", node.Name)
	l.Info("Received new node")
	l.Infof("  PublicKey: %v", publicKey)
	l.Infof("  Address: %v", addr)
	l.Infof("  PodCIDR: %v", cidr)

	if publicKey == "" || addr == "" || cidr == "" {
		// node is missing a required field
		l.Info("Required field missing, ignoring update")
		return
	}

	// TODO(knisbet) remove port hard code
	wireguard.AddPeer(d.Iface, publicKey, d.sharedKey, cidr, addr, "9806")

}

func (d *Daemon) handleNodeUpdated(oldObj interface{}, newObj interface{}) {
	oldNode, oOk := oldObj.(*v1.Node)
	newNode, nOk := newObj.(*v1.Node)
	if !oOk || !nOk {
		d.Warnf("handleNodeUpdated received unexpected object old: %T new: %T", oldObj, newObj)
		return
	}

	oldPublicKey := getNodePublicKey(oldNode)
	newPublicKey := getNodePublicKey(newNode)

	oldAddr := getNodeIP(oldNode)
	newAddr := getNodeIP(newNode)

	oldCidr := oldNode.Spec.PodCIDR
	newCidr := newNode.Spec.PodCIDR

	pubKeyChanged := oldPublicKey != newPublicKey
	cidrChanged := oldCidr != newCidr
	addrChanged := oldAddr != newAddr

	if !pubKeyChanged && !cidrChanged && !addrChanged {
		// nothing we're interested in changed, so just skip processing
		return
	}

	// if the object is for our own node
	// skip processing it
	if newNode.Name == d.NodeName {
		return
	}

	l := d.WithField("node", newNode.Name)
	l.Infof("Received Node Update %v: ", newNode.Name)
	l.Infof("  PublicKey: %v -> %v", oldPublicKey, newPublicKey)
	l.Infof("  Address: %v -> %v", oldAddr, newAddr)
	l.Infof("  PodCIDR: %v -> %v", oldCidr, newCidr)

	// if the public key has been removed from the node
	// delete it from us as well
	if oldPublicKey != "" && newPublicKey == "" {
		l.Info("Removing peer %v/%v", oldNode.Name, oldPublicKey)

		err := wireguard.RemovePeer(d.Iface, oldPublicKey)
		if err != nil {
			l.Warn("Error removing peer: ", trace.DebugReport(err))
		}
		return
	}

	if newPublicKey == "" || newAddr == "" || newCidr == "" {
		// node is missing a required field
		l.Info("Required field missing, ignoring update")
		return
	}

	//TODO(knisbet) remove port hardcode
	if oldPublicKey != "" {
		wireguard.RemovePeer(d.Iface, oldPublicKey)
	}
	wireguard.AddPeer(d.Iface, newPublicKey, d.sharedKey, newCidr, newAddr, "9806")

}

func (d *Daemon) handleNodeDeleted(obj interface{}) {
	node, ok := obj.(*v1.Node)
	if !ok {
		d.Warnf("handleNodeDeleted received unexpected object: %T", obj)
		return
	}

	l := d.WithField("node", node.Name)

	publicKey := getNodePublicKey(node)

	l.Info("Removing peer %v/%v", node.Name, publicKey)

	err := wireguard.RemovePeer(d.Iface, publicKey)
	if err != nil {
		l.Warn("Error removing peer: ", trace.DebugReport(err))
	}
}

func getNodeIP(node *v1.Node) string {
	for _, addr := range node.Status.Addresses {
		// try and parse addr as an IPv4 address
		ip := net.ParseIP(addr.Address)
		if ip == nil || ip.To4() == nil {
			continue
		}

		switch addr.Type {
		case "InternalIP", "ExternalIP":
			return ip.String()
		}
	}
	return ""
}

func getNodeAddr(node *v1.Node) (net.UDPAddr, error) {
	p, ok := node.Annotations[annotationWireguardPort]
	if !ok {
		return net.UDPAddr{}, trace.BadParameter("Node %v missing '%v' annotation", node.Name, annotationWireguardPort)
	}
	port, err := strconv.Atoi(p)
	if err != nil {
		return net.UDPAddr{}, trace.Wrap(err)
	}

	var (
		internalIP net.IP
		externalIP net.IP
	)

	for _, addr := range node.Status.Addresses {
		// try and parse addr as an IPv4 address
		ip := net.ParseIP(addr.Address)
		if ip == nil || ip.To4() == nil {
			continue
		}

		switch addr.Type {
		case "InternalIP":
			internalIP = ip
		case "ExternalIP":
			externalIP = ip
		}
	}

	switch {
	case internalIP != nil:
		return net.UDPAddr{IP: internalIP, Port: port}, nil
	case externalIP != nil:
		return net.UDPAddr{IP: externalIP, Port: port}, nil
	default:
		return net.UDPAddr{}, trace.BadParameter("Node %v unable to find IP Address: ", node.Name)
	}

}

func getNodePublicKey(node *v1.Node) string {
	if key, ok := node.Annotations[annotationWireguardPublicKey]; ok {
		return key
	}
	return ""
}

func (d *Daemon) waitForNodeControllerSync(ctx context.Context) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if d.nodeController.HasSynced() {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}

}

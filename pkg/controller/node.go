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
	"bytes"
	"fmt"
	"net"
	"strconv"
	"text/template"

	"github.com/gravitational/trace"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	annotationWireguardPublicKey = "wireguard-public-key"
	annotationWireguardPort      = "wireguard-port"
)

var annotationPatchTemplate = `[
	{"op": "{{.KeyOp}}", "path": "/metadata/annotations/wireguard-public-key", "value":"{{.KeyValue}}"},
	{"op": "{{.PortOp}}", "path": "/metadata/annotations/wireguard-port", "value":"{{.PortValue}}"}
]`

func (d *Controller) publishPublicKey() error {
	node, err := d.nodeClient.CoreV1().Nodes().Get(d.NodeName, metav1.GetOptions{})
	if err != nil {
		return trace.Wrap(err)
	}

	op := struct {
		KeyOp     string
		KeyValue  string
		PortOp    string
		PortValue string
	}{
		KeyOp:     "add",
		KeyValue:  d.publicKey,
		PortOp:    "add",
		PortValue: fmt.Sprint(d.Port),
	}

	if _, ok := node.Annotations[annotationWireguardPublicKey]; ok {
		op.KeyOp = "replace"
	}
	if _, ok := node.Annotations[annotationWireguardPort]; ok {
		op.PortOp = "replace"
	}

	tmpl, err := template.New("annotationPatch").Parse(annotationPatchTemplate)
	if err != nil {
		return trace.Wrap(err)
	}
	var patch bytes.Buffer
	err = tmpl.Execute(&patch, op)
	if err != nil {
		return trace.Wrap(err)
	}

	// technically this isn't race free
	// if another client adds/removes this annotation between the read/patch it could
	// generate a patching error.
	// but it's acceptable for now

	d.WithField("node", d.NodeName).Info("Publishing public key: ", patch.String())
	_, err = d.nodeClient.CoreV1().Nodes().Patch(d.NodeName, types.JSONPatchType, patch.Bytes())
	if err != nil {
		return trace.Wrap(err)
	}

	d.Debug("Publishing public key complete")

	return nil
}

func (d *Controller) updateNodeNameFromPod(podName, podNamespace string) error {
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

func (d *Controller) getPodCIDR() error {
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

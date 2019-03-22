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
	"net"

	"github.com/gravitational/trace"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ipamInfo struct {
	bridgeAddr    string
	wireguardAddr string
	podAddrStart  string
	podAddrEnd    string
}

func (d *controller) detectIPAM() error {
	d.Debug("Attempting to retrieve IPAM info from k8s IPAM.")

	node, err := d.client.CoreV1().Nodes().Get(d.config.NodeName, metav1.GetOptions{})
	if err != nil {
		return trace.Wrap(err)
	}

	if node.Spec.PodCIDR == "" {
		return trace.BadParameter("node/%v node.spec.podCidr is missing", d.config.NodeName)
	}

	_, _, err = net.ParseCIDR(node.Spec.PodCIDR)
	if err != nil {
		return trace.Wrap(err)
	}

	d.config.NodeCIDR = node.Spec.PodCIDR

	if d.config.Endpoint == "" {
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
			d.config.Endpoint = internalIP.String()
		case externalIP != nil:
			d.config.Endpoint = externalIP.String()
		}
	}

	return nil
}

func (d *controller) calculateIPAMOffsets() error {
	cidr := d.config.NodeCIDR

	_, ipv4Net, err := net.ParseCIDR(cidr)
	if err != nil {
		return trace.Wrap(err)
	}

	// only ipv4 is currently supported
	if ipv4Net.IP.To4() == nil {
		return trace.BadParameter("%v is not an ipv4 subnet", cidr)
	}

	// expect assigned mask to be <= 24 bits
	bits, _ := ipv4Net.Mask.Size()
	if bits > 24 {
		return trace.BadParameter("podCIDR needs to be at least 24 bits. %v is only %v bits.", cidr, bits)
	}

	// .1 for bridge address
	bridgeAddr := net.IP(append([]byte(nil), ipv4Net.IP.To4()...))
	bridgeAddr[3]++

	// .2/32 for wireguard interface
	wireguardAddr := net.IPNet{
		IP:   net.IP(append([]byte(nil), ipv4Net.IP.To4()...)),
		Mask: []byte{255, 255, 255, 255}, // /32
	}
	wireguardAddr.IP[3] += 2

	// .10 for pod IP range start
	rangeStart := net.IP(append([]byte(nil), ipv4Net.IP.To4()...))
	rangeStart[3] += 10

	// .210 for pod IP range end
	rangeEnd := net.IP(append([]byte(nil), ipv4Net.IP.To4()...))
	rangeEnd[3] += 210

	d.ipamInfo = &ipamInfo{
		bridgeAddr:    bridgeAddr.String(),
		wireguardAddr: wireguardAddr.String(),
		podAddrStart:  rangeStart.String(),
		podAddrEnd:    rangeEnd.String(),
	}
	return nil
}

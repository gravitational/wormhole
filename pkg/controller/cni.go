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
	"encoding/json"
	"io/ioutil"
	"path/filepath"

	"github.com/gravitational/trace"
)

func (d *controller) configureCNI() error {
	conf := map[string]interface{}{
		"cniVersion": "0.3.1",
		"name":       "wormhole",
		"plugins": []map[string]interface{}{
			{
				"type":             "bridge",
				"bridge":           d.config.BridgeIface,
				"isGateway":        true,
				"isDefaultGateway": true,
				"forceAddress":     false,
				"ipMasq":           false,
				"hairpinMode":      true,
				// TODO(knisbet)
				// Investigate what MTU setting to use. There are a few things to consider:
				//   - 65535 is the maximum mtu that can be set on a bridge
				//   - This depends significantly, on how the linux kernel represents packets as they pass between
				//     network namespaces and through the linux bridge. If they'rerepresented as ethernet packets,
				//     a large mtu should allow pod-to-pod within a host to be more efficient
				//   - Wireguard implements it's own segmentation, and indicates to the linux kernel that it supports
				//     generic segmentation offload (https://www.wireguard.com/papers/wireguard.pdf section 7.1). If
				//     the bridge MTU plays into this, again, having a large mtu should be more efficient for pod-to-pod
				//     traffic between hosts.
				//   - If the network driver supports/has segmentation offload enabled, having large internal frames
				//     should also be more efficient. So pod -> internet traffic is segmented by the ethernet hardware.
				//   - Also need to check into, whether we're getting a correct MSS, all of this is wasted if we're
				//     using a standard MSS in the TCP handshake
				"mtu": 65535,
				"ipam": map[string]interface{}{
					"type": "host-local",
					"ranges": [][]map[string]string{
						{
							{
								"subnet":     d.config.NodeCIDR,
								"rangeStart": d.ipamInfo.podAddrStart,
								"rangeEnd":   d.ipamInfo.podAddrEnd,
							},
						},
					},
				},
			},
			{
				"type": "portmap",
				"capabilities": map[string]interface{}{
					"portMappings": true,
				},
			},
		},
	}

	jsonConf, err := json.MarshalIndent(conf, "", "    ")
	if err != nil {
		return trace.Wrap(err)
	}

	path := "/etc/cni/net.d/wormhole.conflist"
	if runningInPod() {
		path = filepath.Join("/host", path)
	}
	err = ioutil.WriteFile(path, jsonConf, 0644)
	if err != nil {
		return trace.Wrap(err)
	}

	d.Info("Generated CNI Configuration: ", string(jsonConf))
	return nil
}

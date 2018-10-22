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

func (d *Daemon) configureCNI() error {
	conf := map[string]interface{}{
		"cniVersion": "0.3.1",
		"name":       "wormhole",
		"plugins": []map[string]interface{}{
			{
				"type":             "bridge",
				"bridge":           d.BridgeIface,
				"isGateway":        true,
				"isDefaultGateway": true,
				"forceAddress":     false,
				"ipMasq":           false,
				"hairpinMode":      true,
				"mtu":              65535, // TODO(knisbet) is setting max MTU on the bridge wise?
				"ipam": map[string]interface{}{
					"type": "host-local",
					"ranges": [][]map[string]string{
						{
							{
								"subnet":     d.nodePodCIDR,
								"rangeStart": d.podRangeStart,
								"rangeEnd":   d.podRangeEnd,
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

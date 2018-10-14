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
	"encoding/json"
	"io/ioutil"

	"github.com/gravitational/trace"
)

func (d *Daemon) configureCNI() error {
	conf := map[string]interface{}{
		"cniVersion": "0.3.1",
		"name":       "wormhole",
		"plugins": []map[string]interface{}{
			{
				"type":             "bridge",
				"bridge":           "wormhole-br0",
				"isGateway":        true,
				"isDefaultGateway": true,
				"forceAddress":     false,
				"ipMasq":           false,
				"hairpinMode":      true,
				"mtu":              65535, // TODO(knisbet) properly detect MTU
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
		},
	}

	jsonConf, err := json.MarshalIndent(conf, "", "    ")
	if err != nil {
		return trace.Wrap(err)
	}

	err = ioutil.WriteFile("/etc/cni/net.d/wormhole.conflist", jsonConf, 0644)
	if err != nil {
		return trace.Wrap(err)
	}

	d.Info("Generated CNI Configuration: ", string(jsonConf))
	return nil
}

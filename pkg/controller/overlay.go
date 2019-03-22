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
	"bufio"
	"bytes"
	"io/ioutil"
	"net"
	"strconv"
	"strings"

	"github.com/gravitational/trace"
	yaml "gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// detectOverlayCidr tries and finds the Overlay CIDR network from the kubernetes API
func (d *controller) detectOverlayCidr() error {
	if d.config.OverlayCIDR != "" {
		return nil
	}

	d.Info("Attempting to retrieve overlayCIDR from cluster")

	// Assume we're running inside planet, and continue to other checks if we're not
	cidr, err := d.loadOverlayCidrFromPlanet()
	if err != nil {
		d.Info("Unable to load overlay network from planet: ", trace.DebugReport(err))
	} else {
		d.config.OverlayCIDR = cidr
		return nil
	}

	// try and load config from kubeadm
	cidr, err = loadOverlayCidrFromKubeadm(d.client)
	if err != nil {
		return trace.Wrap(err)
	}
	d.config.OverlayCIDR = cidr
	return nil
}

type kubeadmClusterConfiguration struct {
	Networking map[string]string `yaml:"networking"`
}

// loadOverlayCidrFromKubeadm attempts to load kubeadm installation configuration to discover the overlay network
// subnet in use.
func loadOverlayCidrFromKubeadm(client kubernetes.Interface) (string, error) {
	config, err := client.CoreV1().ConfigMaps("kube-system").Get("kubeadm-config", metav1.GetOptions{})
	if err != nil {
		return "", trace.Wrap(err)
	}

	if _, ok := config.Data["ClusterConfiguration"]; !ok {
		return "", trace.BadParameter("kubeadm configmap is missing ClusterConfiguration")
	}

	var parsedConfig kubeadmClusterConfiguration
	err = yaml.Unmarshal([]byte(config.Data["ClusterConfiguration"]), &parsedConfig)
	if err != nil {
		return "", trace.Wrap(err)
	}

	if cidr, ok := parsedConfig.Networking["podSubnet"]; ok {
		_, _, err := net.ParseCIDR(cidr)
		if err != nil {
			return "", trace.Wrap(err)
		}
		return cidr, nil
	}

	return "", trace.BadParameter("Unable to locate networking.podSubnet in kubeadm config: %v",
		config.Data["ClusterConfiguration"])
}

// loadOverlayCidrFromPlanet attempts to load planet subnet information from the planet /etc/container-environment file
func (d *controller) loadOverlayCidrFromPlanet() (string, error) {
	d.Info("Attempting to retrieve overlayCIDR from planet")
	env, err := ioutil.ReadFile("/etc/container-environment")
	if err != nil {
		return "", trace.ConvertSystemError(err)
	}

	return parsePodSubnetFromPlanet(env)
}

func parsePodSubnetFromPlanet(buf []byte) (string, error) {
	var err error

	scanner := bufio.NewScanner(bytes.NewReader(buf))
	for scanner.Scan() {
		keyVal := strings.SplitN(scanner.Text(), "=", 2)
		if len(keyVal) != 2 {
			continue
		}

		if strings.TrimSpace(keyVal[0]) == "KUBE_POD_SUBNET" {
			val := strings.TrimSpace(keyVal[1])
			if val[0] == '"' {
				val, err = strconv.Unquote(val)
				if err != nil {
					return "", trace.Wrap(err)
				}
			}

			_, _, err = net.ParseCIDR(val)
			if err != nil {
				return "", trace.Wrap(err)
			}
			return val, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", trace.Wrap(err)
	}
	return "", trace.NotFound("Unable to locate KUBE_POD_SUBET env variable in /host/etc/container-environment")
}

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
	"github.com/gravitational/trace"
	yaml "gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// loadOverlayCidr tries and finds the Overlay CIDR network from the kubernetes API
func (d *Controller) loadOverlayCidr() error {
	if d.OverlayCIDR != "" {
		return nil
	}
	d.Info("Attempting to retrieve overlayCIDR from cluster")
	return trace.Wrap(d.loadOverlayCidrFromKubeadm())

}

type kubeadmClusterConfiguration struct {
	Networking map[string]string `yaml:"networking`
}

func (d *Controller) loadOverlayCidrFromKubeadm() error {
	d.Info("Attempting to retrieve overlayCIDR from kubeadm")
	config, err := d.wormholeClient.CoreV1().ConfigMaps("kube-system").Get("kubeadm-config", metav1.GetOptions{})
	if err != nil {
		return trace.Wrap(err)
	}

	if _, ok := config.Data["ClusterConfiguration"]; !ok {
		return trace.BadParameter("kubeadm configmap is missing ClusterConfiguration")
	}

	var parsedConfig kubeadmClusterConfiguration
	err = yaml.Unmarshal([]byte(config.Data["ClusterConfiguration"]), &parsedConfig)
	if err != nil {
		return trace.Wrap(err)
	}

	d.Info("Parsed config: ", parsedConfig)

	if cidr, ok := parsedConfig.Networking["podSubnet"]; ok {
		d.OverlayCIDR = cidr
		return nil
	}

	return trace.BadParameter("Unable to locate networking.podSubnet in kubeadm config: %v",
		config.Data["ClusterConfiguration"])
}

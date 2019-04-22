/*
Copyright 2019 Gravitational, Inc.
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
	"testing"

	"github.com/stretchr/testify/assert"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	testclient "k8s.io/client-go/kubernetes/fake"
)

func TestKubeadmClusterConfiguration(t *testing.T) {
	cases := []struct {
		in  *v1.ConfigMap
		out string
	}{
		{
			in: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kubeadm-config",
					Namespace: "kube-system",
				},
				Data: map[string]string{
					"ClusterConfiguration": `
networking:
  dnsDomain: cluster.local
  podSubnet: 10.20.0.0/16
  serviceSubnet: 10.99.0.0/24`,
				},
			},
			out: "10.20.0.0/16",
		},
	}

	for _, c := range cases {
		client := testclient.NewSimpleClientset(c.in)
		cidr, err := loadOverlayCidrFromKubeadm(client)
		assert.NoError(t, err, c.out)
		assert.Equal(t, c.out, cidr, c.out)
	}

}

func TestKubeadmClusterConfigurationErrors(t *testing.T) {
	cases := []struct {
		in          *v1.ConfigMap
		description string
	}{
		{
			in:          &v1.ConfigMap{},
			description: "empty configmap",
		},
		{
			in: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kubeadm-config",
					Namespace: "kube-system",
				},
			},
			description: "empty data",
		},
		{
			in: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kubeadm-config",
					Namespace: "kube-system",
				},
				Data: map[string]string{
					"ClusterConfiguration": `
derp: true`,
				},
			},
			description: "missing networking",
		},
		{
			in: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kubeadm-config",
					Namespace: "kube-system",
				},
				Data: map[string]string{
					"ClusterConfiguration": `
networking:
  dnsDomain: cluster.local
  podSubnet: broken
  serviceSubnet: 10.99.0.0/24`,
				},
			},
			description: "invalid cidr",
		},
		{
			in: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kubeadm-config",
					Namespace: "kube-system",
				},
				Data: map[string]string{
					"ClusterConfiguration": `
	networking:
	`,
				},
			},
			description: "invalid yaml",
		},
	}

	for _, c := range cases {
		client := testclient.NewSimpleClientset(c.in)
		_, err := loadOverlayCidrFromKubeadm(client)
		assert.Error(t, err, c.description)
	}

}

func TestPlanetConfiguration(t *testing.T) {
	cases := []struct {
		env         string
		expected    string
		description string
	}{
		{
			env:         `KUBE_POD_SUBNET="10.20.0.0/16"`,
			expected:    "10.20.0.0/16",
			description: "simple file",
		},
		{
			env:         `KUBE_POD_SUBNET=10.20.0.0/16`,
			expected:    "10.20.0.0/16",
			description: "unquoted",
		},
		{
			env: `
			TEST=TEST
			TEST=
			KUBE_POD_SUBNET = "10.20.0.0/16"
			TEST=TEST
			TEST
			`,
			expected:    "10.20.0.0/16",
			description: "multi line",
		},
	}

	for _, c := range cases {
		cidr, err := parsePodSubnetFromPlanet([]byte(c.env))
		assert.NoError(t, err, c.description)
		assert.Equal(t, c.expected, cidr, c.description)
	}
}

func TestPlanetConfigurationError(t *testing.T) {
	cases := []struct {
		env         string
		description string
	}{
		{
			env:         `KUBE_POD_SUBNET="10.20.0.0/16`,
			description: "missing quote",
		},
		{
			env:         `KUBE_POD_SUBNET`,
			description: "missing value",
		},
		{
			env:         `KUBE_POD_SUBNET="test"`,
			description: "invalid cidr",
		},
	}

	for _, c := range cases {
		_, err := parsePodSubnetFromPlanet([]byte(c.env))
		assert.Error(t, err, c.description)
	}
}

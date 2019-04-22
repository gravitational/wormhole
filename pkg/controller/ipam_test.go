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

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	testclient "k8s.io/client-go/kubernetes/fake"
)

func TestDetectIPAM(t *testing.T) {
	cases := []struct {
		description string
		node        *v1.Node
		expected    string
		expectErr   bool
	}{
		{
			description: "pod cidr set",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
				Spec: v1.NodeSpec{
					PodCIDR: "10.20.1.0/24",
				},
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{
						{
							Type:    "InternalIP",
							Address: "10.0.0.1",
						},
					},
				},
			},
			expected: "10.20.1.0/24",
		},
		{
			description: "pod cidr malformed",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
				Spec: v1.NodeSpec{
					PodCIDR: "10.20.1.",
				},
			},
			expectErr: true,
		},
		{
			description: "pod cidr missing",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
			},
			expectErr: true,
		},
	}

	for _, c := range cases {
		cont := &controller{
			FieldLogger: logrus.WithField("logger", "test"),
			client:      testclient.NewSimpleClientset(c.node),
			config: Config{
				NodeName: c.node.Name,
			},
		}
		err := cont.detectIPAM()
		if c.expectErr {
			assert.Error(t, err, c.description)
		} else {
			assert.NoError(t, err, c.description)
			assert.Equal(t, c.expected, cont.config.NodeCIDR, c.description)
		}
	}
}

func TestDetectIPAMNodeAddress(t *testing.T) {
	cases := []struct {
		node     *v1.Node
		expected string
	}{
		{
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
				Spec: v1.NodeSpec{
					PodCIDR: "10.20.1.0/24",
				},
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{
						{
							Type:    "InternalIP",
							Address: "10.0.0.1",
						},
					},
				},
			},
			expected: "10.0.0.1",
		},
		{
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
				Spec: v1.NodeSpec{
					PodCIDR: "10.20.1.0/24",
				},
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{
						{
							Type:    "ExternalIP",
							Address: "10.0.0.2",
						},
					},
				},
			},
			expected: "10.0.0.2",
		},
		{
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
				Spec: v1.NodeSpec{
					PodCIDR: "10.20.1.0/24",
				},
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{
						{
							Type:    "InternalIP",
							Address: "10.0.0.3",
						},
						{
							Type:    "ExternalIP",
							Address: "10.0.0.4",
						},
					},
				},
			},
			expected: "10.0.0.3",
		},
		{
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
				Spec: v1.NodeSpec{
					PodCIDR: "10.20.1.0/24",
				},
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{
						{
							Type:    "InternalIP",
							Address: "::1",
						},
						{
							Type:    "ExternalIP",
							Address: "::2",
						},
						{
							Type:    "InternalIP",
							Address: "10.0.0.5",
						},
						{
							Type:    "ExternalIP",
							Address: "10.0.0.6",
						},
					},
				},
			},
			expected: "10.0.0.5",
		},
		{
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
				Spec: v1.NodeSpec{
					PodCIDR: "10.20.1.0/24",
				},
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{},
				},
			},
			expected: "",
		},
	}

	for _, c := range cases {
		cont := &controller{
			FieldLogger: logrus.WithField("logger", "test"),
			client:      testclient.NewSimpleClientset(c.node),
			config: Config{
				NodeName: c.node.Name,
			},
		}
		err := cont.detectIPAM()

		assert.NoError(t, err, c.expected)
		assert.Equal(t, c.expected, cont.config.Endpoint, c.expected)

	}
}

func TestIPAMOffsets(t *testing.T) {
	cases := []struct {
		in        string
		expected  *ipamInfo
		expectErr bool
	}{
		{
			in: "10.20.0.0/24",
			expected: &ipamInfo{
				bridgeAddr:    "10.20.0.1",
				wireguardAddr: "10.20.0.2/32",
				podAddrStart:  "10.20.0.10",
				podAddrEnd:    "10.20.0.210",
			},
		},
		{
			in:        "10.20.0",
			expectErr: true,
		},
		{
			in:        "10.20.0.0/25",
			expectErr: true,
		},
		{
			in:        "::1/64",
			expectErr: true,
		},
	}

	for _, c := range cases {
		cont := &controller{
			FieldLogger: logrus.WithField("logger", "test"),
			config: Config{
				NodeCIDR: c.in,
			},
		}
		err := cont.calculateIPAMOffsets()
		if c.expectErr {
			assert.Error(t, err, c.in)
		} else {
			assert.NoError(t, err, c.in)
			assert.Equal(t, c.expected, cont.ipamInfo, c.in)
		}
	}
}

/*
func TestLoadIPAM(t *testing.T) {
	cases := []struct {
		node        *v1.Node
		description string
		expected    *ipamInfo
	}{
		{
			description: "test",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
				Spec: v1.NodeSpec{
					PodCIDR: "10.20.1.0/24",
				},
			},
			expected: &ipamInfo{
				bridgeAddr:    "10.20.1.1",
				wireguardAddr: "10.20.1.2/32",
				podAddrStart:  "10.20.1.10",
				podAddrEnd:    "10.20.1.210",
			},
		},
	}

	for _, c := range cases {
		cont := &controller{
			FieldLogger: logrus.WithField("logger", "test"),
			client:      testclient.NewSimpleClientset(c.node),
			config: Config{
				NodeName: c.node.Name,
			},
		}
		ipam, err := cont.loadIPAM()
		assert.NoError(t, err, c.description)
		assert.Equal(t, c.expected, ipam, c.description)
	}

}

func TestLoadIPAMError(t *testing.T) {
	cases := []struct {
		node        *v1.Node
		nodeName    string
		description string
	}{
		{
			description: "empty pod cidr",
			nodeName:    "test-node",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
			},
		},
		{
			description: "missing bad node name",
			nodeName:    "test-missing-...",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
			},
		},
		{
			description: "bad cidr",
			nodeName:    "test-node",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
				Spec: v1.NodeSpec{
					PodCIDR: "10.20.1.0/24343",
				},
			},
		},
		{
			description: "ipv6",
			nodeName:    "test-node",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
				Spec: v1.NodeSpec{
					PodCIDR: "::1/64",
				},
			},
		},
		{
			description: "small subnet",
			nodeName:    "test-node",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
				Spec: v1.NodeSpec{
					PodCIDR: "10.20.1.0/25",
				},
			},
		},
	}

	for _, c := range cases {
		cont := &controller{
			FieldLogger: logrus.WithField("logger", "test"),
			client:      testclient.NewSimpleClientset(c.node),
			config: Config{
				NodeName: c.node.Name,
			},
		}
		_, err := cont.loadIPAM()
		assert.Error(t, err, c.description)
	}

}
*/

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
	"testing"

	"github.com/gravitational/wormhole/pkg/wireguard"
	"github.com/sirupsen/logrus"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	testclient "k8s.io/client-go/kubernetes/fake"
)

func TestInitKubeObjects(t *testing.T) {
	c := controller{
		FieldLogger: logrus.New(),
		client:      testclient.NewSimpleClientset(),
	}

	err := c.initKubeObjects()
	assert.NoError(t, err, "first attempt")

	err = c.initKubeObjects()
	assert.NoError(t, err, "second attempt")
}

func TestPublishPublicKey(t *testing.T) {

	cases := []struct {
		publicKey string
		nodeName  string
		port      int
		namespace string
		expected  map[string][]byte
	}{
		{
			namespace: "test-1",
			publicKey: "public-key",
			nodeName:  "node-1",
			port:      1000,
			expected: map[string][]byte{
				"publickey-node-1": []byte("public-key"),
				"port-node-1":      []byte("1000"),
			},
		},
	}

	for _, tt := range cases {

		c := controller{
			FieldLogger: logrus.New(),
			client:      testclient.NewSimpleClientset(),
			wireguardInterface: mockWireguardInterface{
				publicKey: tt.publicKey,
			},
			config: Config{
				NodeName:  tt.nodeName,
				Port:      tt.port,
				Namespace: tt.namespace,
			},
		}

		_ = c.initKubeObjects()

		err := c.publishPublicKey()
		assert.NoError(t, err, "first attempt")

		err = c.publishPublicKey()
		assert.NoError(t, err, "second attempt")

		configMap, err := c.client.CoreV1().ConfigMaps(tt.namespace).Get(kubeConfigMapPublic, metav1.GetOptions{})
		assert.NoError(t, err, tt.namespace)
		assert.Equal(t, tt.expected, configMap.BinaryData, tt.namespace)
	}
}

type mockWireguardInterface struct {
	publicKey string
}

func (m mockWireguardInterface) PublicKey() string {
	return m.publicKey
}

func (mockWireguardInterface) SyncPeers(map[string]wireguard.Peer) {

}

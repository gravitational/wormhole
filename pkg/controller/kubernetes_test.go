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
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gravitational/wormhole/pkg/apis/wormhole.gravitational.io/v1beta1"
	wormholeclientset "github.com/gravitational/wormhole/pkg/client/clientset/versioned/fake"
	"github.com/gravitational/wormhole/pkg/wireguard"
	"github.com/sirupsen/logrus"
	testclient "k8s.io/client-go/kubernetes/fake"
)

func TestPublishNodeInfo(t *testing.T) {
	cases := []struct {
		publicKey string
		nodeName  string
		port      int
	}{
		{
			publicKey: "public-1",
			nodeName:  "node-1",
			port:      1000,
		},
	}

	for _, c := range cases {
		cont := &controller{
			FieldLogger: logrus.WithField("logger", "test"),
			crdClient:   wormholeclientset.NewSimpleClientset(),
			config: Config{
				NodeName: c.nodeName,
				Port:     c.port,
			},
			wireguardInterface: &mockWireguardInterface{
				publicKey: c.publicKey,
			},
		}

		// publish the node multiple times
		err := cont.publishNodeInfo()
		assert.NoError(t, err, c.nodeName)

		err = cont.publishNodeInfo()
		assert.NoError(t, err, c.nodeName)

		err = cont.publishNodeInfo()
		assert.NoError(t, err, c.nodeName)

		crd, err := cont.crdClient.WormholeV1beta1().Nodes().Get(c.nodeName, metav1.GetOptions{})
		assert.NoError(t, err, c.nodeName)
		assert.Equal(t, v1beta1.NodeStatus{
			Port:      c.port,
			PublicKey: c.publicKey,
		}, crd.Status, c.nodeName)
	}
}

func TestGenerateNodeSecret(t *testing.T) {
	cases := []struct {
		peerName  string
		sharedKey string
		expected  map[string][]byte
	}{
		{
			peerName:  "peer1",
			sharedKey: "shared1",
			expected: map[string][]byte{
				"shared-secret-peer1-testNode1": []byte("shared1"),
			},
		},
		{
			peerName:  "peer2",
			sharedKey: "shared2",
			expected: map[string][]byte{
				"shared-secret-peer1-testNode1": []byte("shared1"),
				"shared-secret-peer2-testNode1": []byte("shared2"),
			},
		},
		{
			peerName:  "peer1",
			sharedKey: "shared3",
			expected: map[string][]byte{
				"shared-secret-peer1-testNode1": []byte("shared3"),
				"shared-secret-peer2-testNode1": []byte("shared2"),
			},
		},
	}

	wgi := mockWireguardInterface{}

	cont := &controller{
		FieldLogger: logrus.WithField("logger", "test"),
		client:      testclient.NewSimpleClientset(),
		config: Config{
			NodeName:  "testNode1",
			Namespace: "test",
		},
		wireguardInterface: &wgi,
	}
	logrus.SetLevel(logrus.DebugLevel)
	err := cont.initKubeObjects()
	assert.NoError(t, err, "init kube objects")
	cont.startSecretWatcher(context.TODO())

	for !cont.secretController.HasSynced() {
		time.Sleep(time.Millisecond)
	}

	for _, c := range cases {
		wgi.sharedKey = c.sharedKey
		secret, err := cont.generatePeerSharedSecret(c.peerName)
		assert.NoError(t, err, c.sharedKey)
		assert.Equal(t, c.sharedKey, secret, c.sharedKey)

		// Hack: it's not clear when using the fake kubernetes client, when writing an object, how to deterministically
		// wait for the watcher/lister to get updated. For now, just sleep.
		time.Sleep(time.Millisecond)
		cont.secretController.LastSyncResourceVersion()
		o, err := cont.secretLister.Secrets(cont.config.Namespace).Get(secretObjectName)
		assert.NoError(t, err, c.sharedKey)
		assert.Equal(t, c.expected, o.Data, c.sharedKey)
	}
}

func TestIntegratePeers(t *testing.T) {
	cases := []struct {
		add      []wireguard.Peer
		del      []wireguard.Peer
		expected map[string]wireguard.Peer
	}{
		{
			add: []wireguard.Peer{
				wireguard.Peer{
					PublicKey: "public1",
					SharedKey: "shared1",
					AllowedIP: []string{"10.240.1.0/24"},
					Endpoint:  "10.0.0.1",
				},
			},
			expected: map[string]wireguard.Peer{
				"public1": wireguard.Peer{
					PublicKey: "public1",
					SharedKey: "shared1",
					AllowedIP: []string{"10.240.1.0/24"},
					Endpoint:  "10.0.0.1",
				},
			},
		},
		{
			add: []wireguard.Peer{
				wireguard.Peer{
					PublicKey: "public2",
					SharedKey: "shared2",
					AllowedIP: []string{"10.240.2.0/24"},
					Endpoint:  "10.0.0.2",
				},
			},
			expected: map[string]wireguard.Peer{
				"public1": wireguard.Peer{
					PublicKey: "public1",
					SharedKey: "shared1",
					AllowedIP: []string{"10.240.1.0/24"},
					Endpoint:  "10.0.0.1",
				},
				"public2": wireguard.Peer{
					PublicKey: "public2",
					SharedKey: "shared2",
					AllowedIP: []string{"10.240.2.0/24"},
					Endpoint:  "10.0.0.2",
				},
			},
		},
		{
			del: []wireguard.Peer{
				wireguard.Peer{
					PublicKey: "public2",
				},
			},
			expected: map[string]wireguard.Peer{
				"public1": wireguard.Peer{
					PublicKey: "public1",
					SharedKey: "shared1",
					AllowedIP: []string{"10.240.1.0/24"},
					Endpoint:  "10.0.0.1",
				},
			},
		},
	}

	wgi := mockWireguardInterface{}

	cont := &controller{
		FieldLogger: logrus.WithField("logger", "test"),
		client:      testclient.NewSimpleClientset(),
		crdClient:   wormholeclientset.NewSimpleClientset(),
		config: Config{
			NodeName:  "test-node",
			Namespace: "test",
		},
		wireguardInterface: &wgi,
	}
	logrus.SetLevel(logrus.DebugLevel)
	err := cont.initKubeObjects()
	assert.NoError(t, err, "init kube objects")
	cont.startSecretWatcher(context.TODO())
	cont.startNodeWatcher(context.TODO())

	cont.waitForControllerSync(context.TODO())

	for _, tt := range cases {

		for _, add := range tt.add {
			_, err = cont.crdClient.WormholeV1beta1().Nodes().Create(&v1beta1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: add.PublicKey,
				},
				Status: v1beta1.NodeStatus{
					Port:      1000,
					PublicKey: add.PublicKey,
					NodeCIDR:  add.AllowedIP[0],
					Endpoint:  add.Endpoint,
				},
			})
			assert.NoError(t, err, "%v", add)

			wgi.sharedKey = add.SharedKey
			err = cont.retryGeneratePeerSharedSecret(add.PublicKey)
			assert.NoError(t, err, "%v", tt)
		}

		for _, del := range tt.del {
			err = cont.crdClient.WormholeV1beta1().Nodes().Delete(del.PublicKey, &metav1.DeleteOptions{})
			assert.NoError(t, err, "%v", del)
		}

		// Hack: it's not clear when using the fake kubernetes client, when writing an object, how to deterministically
		// wait for the watcher/lister to get updated. For now, just sleep.
		time.Sleep(5 * time.Millisecond)
		err = cont.syncWithWireguard()
		assert.NoError(t, err, "%v", tt)
		assert.NotNil(t, wgi.peers, "%v", tt)
		assert.Equal(t, tt.expected, wgi.peers, "%v", tt)
	}
}

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

func TestUpdatePeerSecrets(t *testing.T) {
	cases := []struct {
		nodes        []*v1beta1.Node
		secret       *v1.Secret
		sharedSecret string
	}{
		{
			nodes: []*v1beta1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test1",
					},
				},
			},
			secret: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretObjectName,
					Namespace: "test",
				},
				Data: map[string][]byte{
					"shared-secret-test0-test1": []byte("secret1"),
				},
			},
			sharedSecret: "secret1",
		},
		{
			nodes: []*v1beta1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test2",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test3",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test4",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test5",
					},
				},
			},
			secret: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretObjectName,
					Namespace: "test",
				},
				Data: map[string][]byte{
					"shared-secret-test0-test1": []byte("secret2"),
					"shared-secret-test0-test2": []byte("secret2"),
					"shared-secret-test0-test3": []byte("secret2"),
					"shared-secret-test0-test4": []byte("secret2"),
					"shared-secret-test0-test5": []byte("secret2"),
				},
			},
			sharedSecret: "secret2",
		},
	}

	for _, tt := range cases {
		wgi := mockWireguardInterface{
			sharedKey: tt.sharedSecret,
		}

		cont := &controller{
			FieldLogger: logrus.WithField("logger", "test"),
			client:      testclient.NewSimpleClientset(),
			crdClient:   wormholeclientset.NewSimpleClientset(),
			config: Config{
				NodeName:  "test0",
				Namespace: "test",
			},
			wireguardInterface: &wgi,
		}
		err := cont.initKubeObjects()
		assert.NoError(t, err, tt.sharedSecret)

		for _, n := range tt.nodes {
			_, err := cont.crdClient.WormholeV1beta1().Nodes().Create(n)
			assert.NoError(t, err, tt.sharedSecret)
		}

		cont.startSecretWatcher(context.TODO())
		cont.startNodeWatcher(context.TODO())
		cont.waitForControllerSync(context.TODO())

		err = cont.updatePeerSecrets()
		assert.NoError(t, err, tt.sharedSecret)

		secrets, err := cont.client.CoreV1().Secrets("test").Get(secretObjectName, metav1.GetOptions{})
		assert.NoError(t, err, tt.sharedSecret)
		assert.Equal(t, tt.secret.Data, secrets.Data, tt.sharedSecret)
	}

}

type mockWireguardInterface struct {
	publicKey string
	sharedKey string
	peers     map[string]wireguard.Peer
}

func (m mockWireguardInterface) PublicKey() string {
	return m.publicKey
}

func (m *mockWireguardInterface) SyncPeers(peers map[string]wireguard.Peer) {
	m.peers = peers
}

func (m mockWireguardInterface) GenerateSharedKey() (string, error) {
	return m.sharedKey, nil
}
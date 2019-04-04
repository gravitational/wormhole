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
	"sync"
	"testing"
	"time"

	"github.com/gravitational/wormhole/pkg/apis/wormhole.gravitational.io/v1beta1"
	wormholeclientset "github.com/gravitational/wormhole/pkg/client/clientset/versioned/fake"
	"github.com/gravitational/wormhole/pkg/wireguard"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
				NodeName:  c.nodeName,
				Namespace: "test",
				Port:      c.port,
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

		crd, err := cont.crdClient.WormholeV1beta1().Wgnodes("test").Get(c.nodeName, metav1.GetOptions{})
		assert.NoError(t, err, c.nodeName)
		assert.Equal(t, v1beta1.WgnodeStatus{
			Port:      c.port,
			PublicKey: c.publicKey,
		}, crd.Status, c.nodeName)
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
				{
					PublicKey: "public1",
					SharedKey: "shared1",
					AllowedIP: []string{"10.240.1.0/24"},
					Endpoint:  "10.0.0.1",
				},
			},
			expected: map[string]wireguard.Peer{
				"public1": {
					PublicKey: "public1",
					SharedKey: "shared1",
					AllowedIP: []string{"10.240.1.0/24"},
					Endpoint:  "10.0.0.1:1000",
				},
			},
		},
		{
			add: []wireguard.Peer{
				{
					PublicKey: "public2",
					SharedKey: "shared2",
					AllowedIP: []string{"10.240.2.0/24"},
					Endpoint:  "10.0.0.2",
				},
			},
			expected: map[string]wireguard.Peer{
				"public1": {
					PublicKey: "public1",
					SharedKey: "shared1",
					AllowedIP: []string{"10.240.1.0/24"},
					Endpoint:  "10.0.0.1:1000",
				},
				"public2": {
					PublicKey: "public2",
					SharedKey: "shared2",
					AllowedIP: []string{"10.240.2.0/24"},
					Endpoint:  "10.0.0.2:1000",
				},
			},
		},
		{
			del: []wireguard.Peer{
				{
					PublicKey: "public2",
				},
			},
			expected: map[string]wireguard.Peer{
				"public1": {
					PublicKey: "public1",
					SharedKey: "shared1",
					AllowedIP: []string{"10.240.1.0/24"},
					Endpoint:  "10.0.0.1:1000",
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
			NodeName:     "test-node",
			Namespace:    "test",
			SyncInterval: 100 * time.Millisecond,
		},
		wireguardInterface: &wgi,
	}
	logrus.SetLevel(logrus.DebugLevel)
	err := cont.initKubeObjects()
	assert.NoError(t, err, "init kube objects")
	cont.startSecretWatcher(context.TODO())
	cont.startNodeWatcher(context.TODO())

	_ = cont.waitForControllerSync(context.TODO())

	for _, tt := range cases {

		for _, add := range tt.add {
			_, err = cont.crdClient.WormholeV1beta1().Wgnodes("test").Create(&v1beta1.Wgnode{
				ObjectMeta: metav1.ObjectMeta{
					Name: add.PublicKey,
				},
				Status: v1beta1.WgnodeStatus{
					Port:      1000,
					PublicKey: add.PublicKey,
					NodeCIDR:  add.AllowedIP[0],
					Endpoint:  add.Endpoint,
				},
			})
			assert.NoError(t, err, "%v", add)

			wgi.sharedKey = add.SharedKey
		}

		for _, del := range tt.del {
			err = cont.crdClient.WormholeV1beta1().Wgnodes("test").Delete(del.PublicKey, &metav1.DeleteOptions{})
			assert.NoError(t, err, "%v", del)
		}

		// Hack: it's not clear when using the fake kubernetes client, when writing an object, how to deterministically
		// wait for the watcher/lister to get updated. For now, just sleep.
		time.Sleep(5 * time.Millisecond)

		err = cont.updatePeerSecrets(false)
		assert.NoError(t, err, "%v", tt)

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
		nodes        []*v1beta1.Wgnode
		secret       *v1.Secret
		sharedSecret string
		overwrite    bool
	}{
		{
			nodes: []*v1beta1.Wgnode{
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
			overwrite:    true,
		},
		{
			nodes: []*v1beta1.Wgnode{
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
			overwrite:    true,
		},
		{
			nodes: []*v1beta1.Wgnode{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test6",
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
					"shared-secret-test0-test6": []byte("secret3"),
				},
			},
			sharedSecret: "secret3",
			overwrite:    false,
		},
	}

	wgi := mockWireguardInterface{}

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
	assert.NoError(t, err, "kube init")

	for _, tt := range cases {
		wgi.sharedKey = tt.sharedSecret
		for _, n := range tt.nodes {
			_, err := cont.crdClient.WormholeV1beta1().Wgnodes("test").Create(n)
			assert.NoError(t, err, tt.sharedSecret)
		}

		cont.startSecretWatcher(context.TODO())
		cont.startNodeWatcher(context.TODO())
		_ = cont.waitForControllerSync(context.TODO())

		err = cont.updatePeerSecrets(tt.overwrite)
		assert.NoError(t, err, tt.sharedSecret)

		secrets, err := cont.client.CoreV1().Secrets("test").Get(secretObjectName, metav1.GetOptions{})
		assert.NoError(t, err, tt.sharedSecret)
		assert.Equal(t, tt.secret.Data, secrets.Data, tt.sharedSecret)
	}

}

type mockWireguardInterface struct {
	sync.Mutex
	publicKey string
	sharedKey string
	peers     map[string]wireguard.Peer
}

func (m *mockWireguardInterface) PublicKey() string {
	return m.publicKey
}

func (m *mockWireguardInterface) SyncPeers(peers map[string]wireguard.Peer) error {
	m.Lock()
	defer m.Unlock()
	m.peers = peers
	return nil
}

func (m *mockWireguardInterface) GenerateSharedKey() (string, error) {
	return m.sharedKey, nil
}

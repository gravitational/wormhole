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

package wireguard

import (
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"

	"github.com/stretchr/testify/assert"
)

const (
	test = "test"
)

func TestCheckDefaults(t *testing.T) {

	cases := []struct {
		in        Config
		expectErr bool
	}{
		{
			in:        Config{},
			expectErr: true,
		},
		{
			in: Config{
				IP:   "10.2.2.5",
				Port: 100,
			},
			expectErr: true,
		},
		{
			in: Config{
				InterfaceName: test,
				Port:          100,
			},
			expectErr: true,
		},
		{
			in: Config{
				InterfaceName: test,
				IP:            "10.2.2.5/24",
			},
			expectErr: true,
		},
		{
			in: Config{
				InterfaceName: test,
				IP:            "10.2.2.5/24",
				Port:          100,
			},
			expectErr: false,
		},
		{
			in: Config{
				InterfaceName: test,
				IP:            "500.2.2.5/24",
				Port:          100,
			},
			expectErr: true,
		},
		{
			in: Config{
				InterfaceName: test,
				IP:            "::1/24",
				Port:          100,
			},
			expectErr: true,
		},
	}

	for _, c := range cases {
		err := c.in.CheckAndSetDefaults()
		if c.expectErr {
			assert.Error(t, err, spew.Sdump(c.in))
		} else {
			assert.NoError(t, err, spew.Sdump(c.in))
		}
	}

}

func TestNew(t *testing.T) {

	cases := []struct {
		in       Config
		expected mockWg
	}{
		{
			in: Config{
				InterfaceName: "wg0",
				IP:            "10.0.0.0/24",
				Port:          1000,
			},
			expected: mockWg{
				iface:      "wg0",
				privateKey: test,
				ip:         "10.0.0.0/24",
				port:       1000,
				up:         true,
			},
		},
	}

	for _, c := range cases {
		m := mockWg{
			iface: c.in.InterfaceName,
		}
		_, err := new(c.in, &m)
		assert.NoError(t, err, spew.Sdump(c.in))
		assert.Equal(t, c.expected, m, spew.Sdump(c.in))
	}
}

func TestSyncPeers(t *testing.T) {
	peers := []Peer{
		{
			PublicKey: "peer0",
			SharedKey: "shared0",
			AllowedIP: []string{"10.0.0.0/24"},
			Endpoint:  "1.0.0.0",
		},
		{
			PublicKey: "peer1",
			SharedKey: "shared1",
			AllowedIP: []string{"10.0.1.0/24"},
			Endpoint:  "1.0.0.1",
		},
		{
			PublicKey: "peer2",
			SharedKey: "shared2",
			AllowedIP: []string{"10.0.2.0/24"},
			Endpoint:  "1.0.0.2",
		},
	}

	changedPeers := []Peer{
		{
			PublicKey: "peer0",
			SharedKey: "shared0",
			AllowedIP: []string{"10.0.0.0/24"},
			Endpoint:  "1.0.0.0",
		},
		{
			PublicKey: "peer0",
			SharedKey: "shared0",
			AllowedIP: []string{"10.0.0.0/24", "10.1.0.0/24"},
			Endpoint:  "1.0.0.0",
		},
		{
			PublicKey: "peer0",
			SharedKey: "shared0",
			AllowedIP: []string{"10.0.0.0/24"},
			Endpoint:  "1.0.2.0",
		},
	}

	cases := []struct {
		in map[string]Peer
	}{
		{
			in: map[string]Peer{
				"peer0": peers[0],
			},
		},
		{
			in: map[string]Peer{
				"peer0": peers[0],
				"peer1": peers[1],
			},
		},
		{
			in: map[string]Peer{
				"peer0": peers[0],
				"peer1": peers[1],
				"peer2": peers[2],
			},
		},
		{
			in: map[string]Peer{
				"peer0": peers[0],
			},
		},
		{
			in: map[string]Peer{
				"peer0": changedPeers[0],
			},
		},
		{
			in: map[string]Peer{
				"peer0": changedPeers[1],
			},
		},
		{
			in: map[string]Peer{
				"peer0": changedPeers[2],
			},
		},
	}

	m := &mockWg{
		iface: test,
		peers: map[string]Peer{},
	}
	wg0, _ := new(Config{}, m)
	for _, c := range cases {
		wg0.SyncPeers(c.in)
		assert.Equal(t, c.in, m.peers, spew.Sdump(c.in))
	}
}

type mockWg struct {
	iface      string
	privateKey string
	ip         string
	port       int
	up         bool
	err        error
	peers      map[string]Peer
}

func (w *mockWg) genKey() (string, error) {
	if w.err != nil {
		return "", w.err
	}
	return test, nil
}

func (w *mockWg) genPSK() (string, error) {
	if w.err != nil {
		return "", w.err
	}
	return test, nil
}

func (w *mockWg) pubKey(key string) (string, error) {
	if w.err != nil {
		return "", w.err
	}
	return test, nil
}

func (w *mockWg) setPrivateKey(key string) error {
	if w.err != nil {
		return w.err
	}
	w.privateKey = key
	return nil
}

func (w *mockWg) createInterface() error {
	if w.err != nil {
		return w.err
	}
	return nil
}

func (w *mockWg) setIP(ip string) error {
	if w.err != nil {
		return w.err
	}
	w.ip = ip
	return nil
}

func (w *mockWg) setListenPort(port int) error {
	if w.err != nil {
		return w.err
	}
	w.port = port
	return nil
}

func (w *mockWg) setUp() error {
	if w.err != nil {
		return w.err
	}
	w.up = true
	return nil
}

func (w *mockWg) setDown() error {
	if w.err != nil {
		return w.err
	}
	w.up = false
	return nil
}

func (w *mockWg) setRoute(route string) error {
	return nil
}

func (w *mockWg) removePeer(peerPublicKey string) error {
	delete(w.peers, peerPublicKey)
	return nil
}

func (w *mockWg) addPeer(peer Peer) error {
	w.peers[peer.PublicKey] = peer
	return nil
}

func (w *mockWg) getPeers() (map[string]PeerStatus, error) {
	status := map[string]PeerStatus{}
	for _, p := range w.peers {
		status[p.PublicKey] = PeerStatus{
			AllowedIP: strings.Join(p.AllowedIP, ","),
			PublicKey: p.PublicKey,
			Endpoint:  p.Endpoint,
		}
	}
	return status, nil
}

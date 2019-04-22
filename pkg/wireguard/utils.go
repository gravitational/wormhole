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

package wireguard

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gravitational/trace"
	"github.com/magefile/mage/sh"
)

// TODO
// Right now this works by calling the wg command. However, this should be updated to use netlink when possible.

type Wg interface {
	genKey() (string, error)
	genPSK() (string, error)
	pubKey(string) (string, error)
	setPrivateKey(string) error
	createInterface() error
	deleteInterface() error
	setIP(string) error
	setListenPort(int) error
	setUp() error
	setDown() error
	setRoute(string) error
	removePeer(string) error
	addPeer(Peer) error
	getPeers() (map[string]PeerStatus, error)
}

type wg struct {
	sync.Mutex
	iface string

	// sharedSecrets is the map of public-key to shared secrets written to wireguard
	// We currently need to cache this, as the shared secret can't be read back from wireguard
	// and as such it gets missed in our control loop for detecting differences in configuration.
	sharedSecrets map[string]string
}

func (w *wg) genKey() (string, error) {
	key, err := sh.Output("wg", "genkey")
	if err != nil {
		return "", trace.Wrap(err)
	}
	return key, nil
}

func (w *wg) genPSK() (string, error) {
	key, err := sh.Output("wg", "genpsk")
	if err != nil {
		return "", trace.Wrap(err)
	}
	return key, nil
}

func (w *wg) pubKey(key string) (string, error) {
	c := exec.Command("wg", "pubkey")
	c.Env = os.Environ()
	c.Stderr = os.Stderr

	stdin, err := c.StdinPipe()
	if err != nil {
		return "", trace.Wrap(err)
	}

	stdout := &bytes.Buffer{}
	c.Stdout = stdout

	err = c.Start()
	if err != nil {
		return "", trace.Wrap(err)
	}

	_, err = io.WriteString(stdin, key)
	if err != nil {
		return "", trace.Wrap(err)
	}
	stdin.Close()

	err = c.Wait()
	if err != nil {
		return "", trace.Wrap(err)
	}

	return strings.TrimSpace(stdout.String()), nil
}

func (w *wg) setPrivateKey(key string) error {
	// it looks like wireguard only accepts keys by file
	// so we'll need to write the key to a file, load into wireguard
	// then delete it
	tmpFile, err := ioutil.TempFile(os.TempDir(), "")
	if err != nil {
		return trace.Wrap(err)
	}
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.Write([]byte(key))
	if err != nil {
		return trace.Wrap(err)
	}

	return trace.ConvertSystemError(sh.Run(
		"wg",
		"set",
		w.iface,
		"private-key",
		tmpFile.Name(),
	))
}

func (w *wg) createInterface() error {
	buf := &bytes.Buffer{}
	_, err := sh.Exec(
		nil, buf, buf,
		"ip",
		"link",
		"add",
		"dev",
		w.iface,
		"type",
		"wireguard",
	)
	// don't error if the interface already exists
	// TODO(knisbet) look into using netlink so we're not matching error strings
	if strings.Contains(buf.String(), "RTNETLINK answers: File exists") {
		return nil
	}
	return trace.ConvertSystemError(err)
}

func (w *wg) deleteInterface() error {
	buf := &bytes.Buffer{}
	_, err := sh.Exec(
		nil, buf, buf,
		"ip",
		"link",
		"delete",
		"dev",
		w.iface,
	)
	// don't error if the interface doesn't exist
	// TODO(knisbet) look into using netlink so we're not matching error strings
	if strings.Contains(buf.String(), "Cannot find device") {
		return nil
	}
	return trace.ConvertSystemError(err)
}

func (w *wg) setIP(ip string) error {
	buf := &bytes.Buffer{}
	_, err := sh.Exec(
		nil, buf, buf,
		"ip",
		"address",
		"add",
		"dev",
		w.iface,
		ip,
	)
	// don't error if the interface already exists
	if strings.Contains(buf.String(), "RTNETLINK answers: File exists") {
		return nil
	}
	return trace.ConvertSystemError(err)
}

func (w *wg) setListenPort(port int) error {
	return trace.ConvertSystemError(sh.Run(
		"wg",
		"set",
		w.iface,
		"listen-port",
		fmt.Sprint(port),
	))
}

func (w *wg) setUp() error {
	err := sh.Run(
		"ip",
		"link",
		"set",
		"up",
		w.iface,
	)
	// skip if the interface is already up
	if err != nil && strings.Contains(err.Error(), "file exists") {
		return nil
	}
	return trace.ConvertSystemError(err)
}

func (w *wg) setDown() error {
	return trace.ConvertSystemError(sh.Run(
		"ip",
		"link",
		"set",
		"down",
		w.iface,
	))
}

func (w *wg) setRoute(route string) error {
	return trace.ConvertSystemError(sh.Run(
		"ip",
		"route",
		"add",
		route,
		"dev",
		w.iface,
	))
}

func (w *wg) removePeer(peerPublicKey string) error {
	return trace.ConvertSystemError(sh.Run(
		"wg",
		"set",
		w.iface,
		"peer",
		peerPublicKey,
		"remove",
	))
}

func (w *wg) addPeer(peer Peer) error {
	w.Lock()
	defer w.Unlock()
	if w.sharedSecrets == nil {
		w.sharedSecrets = make(map[string]string)
	}
	w.sharedSecrets[peer.PublicKey] = peer.SharedKey

	// it looks like wireguard only accepts keys by file
	// so we'll need to write the key to a file, load into wireguard
	// then delete it
	tmpFile, err := ioutil.TempFile(os.TempDir(), "")
	if err != nil {
		return trace.Wrap(err)
	}
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.Write([]byte(peer.SharedKey))
	if err != nil {
		return trace.Wrap(err)
	}

	return trace.ConvertSystemError(sh.Run(
		"wg",
		"set",
		w.iface,
		"peer",
		peer.PublicKey,
		"allowed-ips",
		strings.Join(peer.AllowedIP, ","),
		"endpoint",
		peer.Endpoint,
		"preshared-key",
		tmpFile.Name(),
		"persistent-keepalive",
		"15",
	))
}

func (w *wg) getPeers() (map[string]PeerStatus, error) {
	o, err := sh.Output(
		"wg",
		"show",
		w.iface,
		"dump",
	)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	results := make(map[string]PeerStatus)

	w.Lock()
	defer w.Unlock()

	for _, line := range strings.Split(o, "\n")[1:] {
		c := strings.Split(line, "\t")
		if len(c) != 8 {
			return nil, trace.BadParameter("Expected 8  columns in wg show but got %v: %v", len(c), c)
		}

		handshakeTime := time.Time{}
		if c[4] != "" {
			i, err := strconv.ParseInt(c[4], 10, 64)
			if err != nil {
				return nil, trace.WrapWithMessage(err, "Error parsing int64: %v", c[4])
			}
			handshakeTime = time.Unix(i, 0)
		}

		bytesRX, err := strconv.ParseInt(c[5], 10, 64)
		if err != nil {
			return nil, trace.WrapWithMessage(err, "Error parsing int64: %v", c[4])
		}
		bytesTX, err := strconv.ParseInt(c[6], 10, 64)
		if err != nil {
			return nil, trace.WrapWithMessage(err, "Error parsing int64: %v", c[4])
		}

		var keepAlive int
		if c[7] != "off" {
			keepAlive, err = strconv.Atoi(c[7])
			if err != nil {
				return nil, trace.WrapWithMessage(err, "Error parsing int64: %v", c[4])
			}
		}

		results[c[0]] = PeerStatus{
			PublicKey:     c[0],
			Endpoint:      replaceNone(c[2]),
			AllowedIP:     replaceNone(c[3]),
			LastHandshake: handshakeTime,
			BytesTX:       bytesTX,
			BytesRX:       bytesRX,
			Keepalive:     keepAlive,
			SharedKey:     w.sharedSecrets[c[0]],
		}

	}

	return results, nil
}

func replaceNone(s string) string {
	if s == "(none)" {
		return ""
	}
	return s
}

func (p PeerStatus) ToPeer() Peer {
	return Peer{
		PublicKey: p.PublicKey,
		SharedKey: p.SharedKey,
		Endpoint:  p.Endpoint,
		AllowedIP: strings.Split(p.AllowedIP, ","),
	}
}

func (p Peer) Equals(r Peer) bool {
	if p.PublicKey != r.PublicKey {
		return false
	}

	if p.SharedKey != r.SharedKey {
		return false
	}

	if len(p.AllowedIP) != len(r.AllowedIP) {
		return false
	}

	sort.Slice(p.AllowedIP, func(i, j int) bool { return p.AllowedIP[i] < p.AllowedIP[j] })
	sort.Slice(r.AllowedIP, func(i, j int) bool { return r.AllowedIP[i] < r.AllowedIP[j] })
	for i := range p.AllowedIP {
		if p.AllowedIP[i] != r.AllowedIP[i] {
			return false
		}
	}

	return true
}

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
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gravitational/trace"
	"github.com/magefile/mage/sh"
)

func GenKey() (string, error) {
	key, err := sh.Output("wg", "genkey")
	if err != nil {
		return "", trace.Wrap(err)
	}
	return key, nil
}

func GenPSK() (string, error) {
	key, err := sh.Output("wg", "genpsk")
	if err != nil {
		return "", trace.Wrap(err)
	}
	return key, nil
}

func PubKey(key string) (string, error) {
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

	return strings.TrimSuffix(stdout.String(), "\n"), nil
}

func SetPrivateKey(iface, key string) error {
	// it looks like wireguard only accepts key's by file
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
		iface,
		"private-key",
		tmpFile.Name(),
	))
}

func CreateInterface(iface string) error {
	return trace.ConvertSystemError(sh.Run(
		"ip",
		"link",
		"add",
		"dev",
		iface,
		"type",
		"wireguard",
	))
}

func SetIP(iface, ip string) error {
	return trace.ConvertSystemError(sh.Run(
		"ip",
		"address",
		"add",
		"dev",
		iface,
		ip,
	))
}

func SetListenPort(iface string, port int) error {
	return trace.ConvertSystemError(sh.Run(
		"wg",
		"set",
		iface,
		"listen-port",
		fmt.Sprint(port),
	))
}

func SetUp(iface string) error {
	return trace.ConvertSystemError(sh.Run(
		"ip",
		"link",
		"set",
		"up",
		iface,
	))
}

func SetDown(iface string) error {
	return trace.ConvertSystemError(sh.Run(
		"ip",
		"link",
		"set",
		"down",
		iface,
	))
}

func SetRoute(iface, route string) error {
	return trace.ConvertSystemError(sh.Run(
		"ip",
		"route",
		"add",
		route,
		"dev",
		iface,
	))
}

func RemovePeer(iface, peerPublicKey string) error {
	return trace.ConvertSystemError(sh.Run(
		"wg",
		"set",
		iface,
		"peer",
		peerPublicKey,
		"remove",
	))
}

func AddPeer(iface, peerPublicKey, sharedKey, subnet, endpoint string) error {
	// it looks like wireguard only accepts key's by file
	// so we'll need to write the key to a file, load into wireguard
	// then delete it
	tmpFile, err := ioutil.TempFile(os.TempDir(), "")
	if err != nil {
		return trace.Wrap(err)
	}
	defer os.Remove(tmpFile.Name())

	return trace.ConvertSystemError(sh.Run(
		"wg",
		"set",
		iface,
		"peer",
		peerPublicKey,
		"allowed-ips",
		subnet,
		"endpoint",
		endpoint,
		"preshared-key",
		tmpFile.Name(),
		"persistent-keepalive",
		"15",
	))
}

type PeerStatus struct {
	PublicKey     string
	Endpoint      string
	AllowedIP     string
	LastHandshake time.Time
	BytesTX       int64
	BytesRX       int64
	Keepalive     int
}

func GetPeerStatus(iface string) (map[string]PeerStatus, error) {
	o, err := sh.Output(
		"wg",
		"show",
		iface,
		"dump",
	)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	results := make(map[string]PeerStatus)

	for _, line := range strings.Split(o, "\n")[1:] {
		c := strings.Split(line, "\t")
		if len(c) != 8 {
			return nil, trace.BadParameter("Unexpected number of columns in wg show: %v", c)
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

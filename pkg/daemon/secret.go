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

package daemon

import (
	"github.com/gravitational/trace"
	"github.com/gravitational/wormhole/pkg/wireguard"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (d Daemon) getOrSetPSK() (string, error) {
	d.Debug("Syncing PSK with cluster.")
	// Secret creation is based on first writer wins.
	// So, generate a new secret, and try and send it to the kubernetes API
	// if we receive an already exists error, then read and return the existing secret
	secret, err := wireguard.GenPSK()
	if err != nil {
		return "", trace.Wrap(err)
	}
	d.Debug("Generating new wireguard PSK as candidate for shared secret.")

	s, err := d.wormholeClient.CoreV1().Secrets("kube-system").Create(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "wormhole",
		},
		StringData: map[string]string{
			"psk": secret,
		},
	})
	if err != nil && !errors.IsAlreadyExists(err) {
		return "", trace.Wrap(err)
	}
	if err != nil {
		d.Debug("Retrieving existing PSK from secret kube-system/wireguard.")
		s, err = d.wormholeClient.CoreV1().Secrets("kube-system").Get("wormhole", metav1.GetOptions{})
		if err != nil {
			return "", trace.Wrap(err)
		}
	}

	binSecret, ok := s.Data["psk"]
	if !ok {
		return "", trace.BadParameter("secret kube-system/wormhole missing psk")
	}

	d.Info("PSK synchronization with cluster complete.")

	return string(binSecret), nil
}

func (d Daemon) generateKeypair() (string, string, error) {
	d.Debug("Generating wireguard keypair.")
	key, err := wireguard.GenKey()
	if err != nil {
		return "", "", trace.Wrap(err)
	}

	pubKey, err := wireguard.PubKey(key)
	if err != nil {
		return "", "", trace.Wrap(err)
	}
	d.WithField("public_key", pubKey).Info("New KeyPair generation complete.")
	return pubKey, key, nil
}

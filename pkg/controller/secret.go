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
	"fmt"
	"time"

	"github.com/davecgh/go-spew/spew"

	"github.com/cenkalti/backoff"

	"github.com/gravitational/trace"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	kubeSecretShared    = "wireguard-shared-secret"
	kubeConfigMapPublic = "wireguard-public-keys"
)

// initKubeObjects runs during startup and is meant to try and create empty kubernetes objects that wormhole uses to
// exchange state via the kubernetes API.
// This just tries to create the object, and ignores already exists errors.
func (c *controller) initKubeObjects() error {
	c.Debug("Initializing secret %v/%v", c.config.Namespace, kubeSecretShared)

	_, err := c.client.CoreV1().Secrets(c.config.Namespace).Create(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: kubeSecretShared,
		},
	})

	if errors.IsAlreadyExists(err) {
		return nil
	}
	if err != nil {
		return trace.Wrap(err)
	}

	c.Debug("Initializing configmap %v/%v", c.config.Namespace, kubeConfigMapPublic)

	_, err = c.client.CoreV1().ConfigMaps(c.config.Namespace).Create(&v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: kubeConfigMapPublic,
		},
	})

	if errors.IsAlreadyExists(err) {
		return nil
	}
	return trace.Wrap(err)
}

func (c *controller) publishPublicKey() error {
	backoff.Retry(func() error {
		c.Debug("Reading configmap")
		m, err := c.client.CoreV1().ConfigMaps(c.config.Namespace).Get(kubeConfigMapPublic, metav1.GetOptions{})
		if err != nil {
			c.WithError(err).Warnf("Error getting configmap %v/%v", c.config.Namespace, kubeConfigMapPublic)
			return trace.Wrap(err)
		}

		if m.BinaryData == nil {
			m.BinaryData = make(map[string][]byte)
		}

		m.BinaryData[fmt.Sprintf("publickey-%v", c.config.NodeName)] = []byte(c.wireguardInterface.PublicKey())
		m.BinaryData[fmt.Sprintf("port-%v", c.config.NodeName)] = []byte(fmt.Sprint(c.config.Port))

		c.Warn(spew.Sdump(m))

		_, err = c.client.CoreV1().ConfigMaps(c.config.Namespace).Update(m)
		if err != nil {
			c.WithError(err).Warnf("Error updating configmap %v/%v", c.config.Namespace, kubeConfigMapPublic)
			return trace.Wrap(err)
		}
		return nil

	}, &backoff.ExponentialBackOff{
		InitialInterval:     1 * time.Second,
		RandomizationFactor: 0.5,
		Multiplier:          2,
		MaxInterval:         30 * time.Second,
		MaxElapsedTime:      300 * time.Second,
		Clock:               backoff.SystemClock,
	})

	c.Debug("Publishing public key complete")

	return nil
}

/*
func (d controller) getOrSetPSK() (string, error) {
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

func (d controller) generateKeypair() (string, string, error) {
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
*/

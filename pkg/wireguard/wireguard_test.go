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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenKey(t *testing.T) {
	// TODO(knisbet) wireguard tools might not be installed when run outside of docker
	_, err := GenKey()
	assert.Nil(t, err)
}

func TestGenPSK(t *testing.T) {
	// TODO(knisbet) wireguard tools might not be installed when run outside of docker
	_, err := GenPSK()
	assert.Nil(t, err)
}

func TestPubKey(t *testing.T) {
	// TODO(knisbet) wireguard tools might not be installed when run outside of docker
	key, err := GenKey()
	assert.Nil(t, err)

	_, err = PubKey(key)
	assert.Nil(t, err)
}

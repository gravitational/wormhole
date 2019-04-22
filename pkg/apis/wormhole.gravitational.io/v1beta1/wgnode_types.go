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

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WgnodeSpec defines the desired state of Wgnode
type WgnodeSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// WgnodeStatus defines the observed state of Wgnode
type WgnodeStatus struct {
	// Port is the port to connect to wireguard on this host
	Port int `json:"port"`
	// PublicKey is the public key of the wireguard node
	PublicKey string `json:"public_key"`
	// NodeCIDR is the IP address range in CIDR format assigned to this node
	NodeCIDR string `json:"node_cidr"`
	// Endpoint is the IP address to connect to this node
	Endpoint string `json:"endpoint"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Wgnode is the Schema for the wgnodes API
// +k8s:openapi-gen=true
type Wgnode struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WgnodeSpec   `json:"spec,omitempty"`
	Status WgnodeStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// WgnodeList contains a list of Wgnode
type WgnodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Wgnode `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Wgnode{}, &WgnodeList{})
}

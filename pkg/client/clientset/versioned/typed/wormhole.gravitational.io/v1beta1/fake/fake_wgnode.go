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
// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	v1beta1 "github.com/gravitational/wormhole/pkg/apis/wormhole.gravitational.io/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeWgnodes implements WgnodeInterface
type FakeWgnodes struct {
	Fake *FakeWormholeV1beta1
	ns   string
}

var wgnodesResource = schema.GroupVersionResource{Group: "wormhole.gravitational.io", Version: "v1beta1", Resource: "wgnodes"}

var wgnodesKind = schema.GroupVersionKind{Group: "wormhole.gravitational.io", Version: "v1beta1", Kind: "Wgnode"}

// Get takes name of the wgnode, and returns the corresponding wgnode object, and an error if there is any.
func (c *FakeWgnodes) Get(name string, options v1.GetOptions) (result *v1beta1.Wgnode, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(wgnodesResource, c.ns, name), &v1beta1.Wgnode{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta1.Wgnode), err
}

// List takes label and field selectors, and returns the list of Wgnodes that match those selectors.
func (c *FakeWgnodes) List(opts v1.ListOptions) (result *v1beta1.WgnodeList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(wgnodesResource, wgnodesKind, c.ns, opts), &v1beta1.WgnodeList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1beta1.WgnodeList{ListMeta: obj.(*v1beta1.WgnodeList).ListMeta}
	for _, item := range obj.(*v1beta1.WgnodeList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested wgnodes.
func (c *FakeWgnodes) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(wgnodesResource, c.ns, opts))

}

// Create takes the representation of a wgnode and creates it.  Returns the server's representation of the wgnode, and an error, if there is any.
func (c *FakeWgnodes) Create(wgnode *v1beta1.Wgnode) (result *v1beta1.Wgnode, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(wgnodesResource, c.ns, wgnode), &v1beta1.Wgnode{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta1.Wgnode), err
}

// Update takes the representation of a wgnode and updates it. Returns the server's representation of the wgnode, and an error, if there is any.
func (c *FakeWgnodes) Update(wgnode *v1beta1.Wgnode) (result *v1beta1.Wgnode, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(wgnodesResource, c.ns, wgnode), &v1beta1.Wgnode{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta1.Wgnode), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeWgnodes) UpdateStatus(wgnode *v1beta1.Wgnode) (*v1beta1.Wgnode, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(wgnodesResource, "status", c.ns, wgnode), &v1beta1.Wgnode{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta1.Wgnode), err
}

// Delete takes name of the wgnode and deletes it. Returns an error if one occurs.
func (c *FakeWgnodes) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(wgnodesResource, c.ns, name), &v1beta1.Wgnode{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeWgnodes) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(wgnodesResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &v1beta1.WgnodeList{})
	return err
}

// Patch applies the patch and returns the patched wgnode.
func (c *FakeWgnodes) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1beta1.Wgnode, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(wgnodesResource, c.ns, name, pt, data, subresources...), &v1beta1.Wgnode{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta1.Wgnode), err
}

/*
Copyright 2020 Authors of Arktos.

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
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeDataPartitionConfigs implements DataPartitionConfigInterface
type FakeDataPartitionConfigs struct {
	Fake *FakeCoreV1
}

var datapartitionconfigsResource = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "datapartitionconfigs"}

var datapartitionconfigsKind = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "DataPartitionConfig"}

// Get takes name of the dataPartitionConfig, and returns the corresponding dataPartitionConfig object, and an error if there is any.
func (c *FakeDataPartitionConfigs) Get(name string, options v1.GetOptions) (result *corev1.DataPartitionConfig, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootGetAction(datapartitionconfigsResource, name), &corev1.DataPartitionConfig{})
	if obj == nil {
		return nil, err
	}

	return obj.(*corev1.DataPartitionConfig), err
}

// List takes label and field selectors, and returns the list of DataPartitionConfigs that match those selectors.
func (c *FakeDataPartitionConfigs) List(opts v1.ListOptions) (result *corev1.DataPartitionConfigList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootListAction(datapartitionconfigsResource, datapartitionconfigsKind, opts), &corev1.DataPartitionConfigList{})
	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &corev1.DataPartitionConfigList{ListMeta: obj.(*corev1.DataPartitionConfigList).ListMeta}
	for _, item := range obj.(*corev1.DataPartitionConfigList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested dataPartitionConfigs.
func (c *FakeDataPartitionConfigs) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewRootWatchAction(datapartitionconfigsResource, opts))
}

// Create takes the representation of a dataPartitionConfig and creates it.  Returns the server's representation of the dataPartitionConfig, and an error, if there is any.
func (c *FakeDataPartitionConfigs) Create(dataPartitionConfig *corev1.DataPartitionConfig) (result *corev1.DataPartitionConfig, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootCreateAction(datapartitionconfigsResource, dataPartitionConfig), &corev1.DataPartitionConfig{})
	if obj == nil {
		return nil, err
	}

	return obj.(*corev1.DataPartitionConfig), err
}

// Update takes the representation of a dataPartitionConfig and updates it. Returns the server's representation of the dataPartitionConfig, and an error, if there is any.
func (c *FakeDataPartitionConfigs) Update(dataPartitionConfig *corev1.DataPartitionConfig) (result *corev1.DataPartitionConfig, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateAction(datapartitionconfigsResource, dataPartitionConfig), &corev1.DataPartitionConfig{})
	if obj == nil {
		return nil, err
	}

	return obj.(*corev1.DataPartitionConfig), err
}

// Delete takes name of the dataPartitionConfig and deletes it. Returns an error if one occurs.
func (c *FakeDataPartitionConfigs) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewRootDeleteAction(datapartitionconfigsResource, name), &corev1.DataPartitionConfig{})
	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeDataPartitionConfigs) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {

	action := testing.NewRootDeleteCollectionAction(datapartitionconfigsResource, listOptions)
	_, err := c.Fake.Invokes(action, &corev1.DataPartitionConfigList{})
	return err
}

// Patch applies the patch and returns the patched dataPartitionConfig.
func (c *FakeDataPartitionConfigs) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *corev1.DataPartitionConfig, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceAction(datapartitionconfigsResource, name, pt, data, subresources...), &corev1.DataPartitionConfig{})
	if obj == nil {
		return nil, err
	}

	return obj.(*corev1.DataPartitionConfig), err
}
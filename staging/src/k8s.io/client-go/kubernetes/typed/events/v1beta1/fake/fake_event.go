/*
Copyright The Kubernetes Authors.
Copyright 2020 Authors of Arktos - file modified.

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
	v1beta1 "k8s.io/api/events/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeEvents implements EventInterface
type FakeEvents struct {
	Fake *FakeEventsV1beta1
	ns   string
	te   string
}

var eventsResource = schema.GroupVersionResource{Group: "events.k8s.io", Version: "v1beta1", Resource: "events"}

var eventsKind = schema.GroupVersionKind{Group: "events.k8s.io", Version: "v1beta1", Kind: "Event"}

// Get takes name of the event, and returns the corresponding event object, and an error if there is any.
func (c *FakeEvents) Get(name string, options v1.GetOptions) (result *v1beta1.Event, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetActionWithMultiTenancy(eventsResource, c.ns, name, c.te), &v1beta1.Event{})

	if obj == nil {
		return nil, err
	}

	return obj.(*v1beta1.Event), err
}

// List takes label and field selectors, and returns the list of Events that match those selectors.
func (c *FakeEvents) List(opts v1.ListOptions) (result *v1beta1.EventList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListActionWithMultiTenancy(eventsResource, eventsKind, c.ns, opts, c.te), &v1beta1.EventList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1beta1.EventList{ListMeta: obj.(*v1beta1.EventList).ListMeta}
	for _, item := range obj.(*v1beta1.EventList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested events.
func (c *FakeEvents) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchActionWithMultiTenancy(eventsResource, c.ns, opts, c.te))

}

// Create takes the representation of a event and creates it.  Returns the server's representation of the event, and an error, if there is any.
func (c *FakeEvents) Create(event *v1beta1.Event) (result *v1beta1.Event, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateActionWithMultiTenancy(eventsResource, c.ns, event, c.te), &v1beta1.Event{})

	if obj == nil {
		return nil, err
	}

	return obj.(*v1beta1.Event), err
}

// Update takes the representation of a event and updates it. Returns the server's representation of the event, and an error, if there is any.
func (c *FakeEvents) Update(event *v1beta1.Event) (result *v1beta1.Event, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateActionWithMultiTenancy(eventsResource, c.ns, event, c.te), &v1beta1.Event{})

	if obj == nil {
		return nil, err
	}

	return obj.(*v1beta1.Event), err
}

// Delete takes name of the event and deletes it. Returns an error if one occurs.
func (c *FakeEvents) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithMultiTenancy(eventsResource, c.ns, name, c.te), &v1beta1.Event{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeEvents) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionActionWithMultiTenancy(eventsResource, c.ns, listOptions, c.te)

	_, err := c.Fake.Invokes(action, &v1beta1.EventList{})
	return err
}

// Patch applies the patch and returns the patched event.
func (c *FakeEvents) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1beta1.Event, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceActionWithMultiTenancy(eventsResource, c.te, c.ns, name, pt, data, subresources...), &v1beta1.Event{})

	if obj == nil {
		return nil, err
	}

	return obj.(*v1beta1.Event), err
}

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

package v1

import (
	strings "strings"
	sync "sync"
	"time"

	v1 "k8s.io/api/core/v1"
	errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	scheme "k8s.io/client-go/kubernetes/scheme"
	rest "k8s.io/client-go/rest"
	klog "k8s.io/klog"
)

// ControllerInstancesGetter has a method to return a ControllerInstanceInterface.
// A group's client should implement this interface.
type ControllerInstancesGetter interface {
	ControllerInstances() ControllerInstanceInterface
}

// ControllerInstanceInterface has methods to work with ControllerInstance resources.
type ControllerInstanceInterface interface {
	Create(*v1.ControllerInstance) (*v1.ControllerInstance, error)
	Update(*v1.ControllerInstance) (*v1.ControllerInstance, error)
	Delete(name string, options *metav1.DeleteOptions) error
	DeleteCollection(options *metav1.DeleteOptions, listOptions metav1.ListOptions) error
	Get(name string, options metav1.GetOptions) (*v1.ControllerInstance, error)
	List(opts metav1.ListOptions) (*v1.ControllerInstanceList, error)
	Watch(opts metav1.ListOptions) watch.AggregatedWatchInterface
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1.ControllerInstance, err error)
	ControllerInstanceExpansion
}

// controllerInstances implements ControllerInstanceInterface
type controllerInstances struct {
	client  rest.Interface
	clients []rest.Interface
}

// newControllerInstances returns a ControllerInstances
func newControllerInstances(c *CoreV1Client) *controllerInstances {
	return &controllerInstances{
		client:  c.RESTClient(),
		clients: c.RESTClients(),
	}
}

// Get takes name of the controllerInstance, and returns the corresponding controllerInstance object, and an error if there is any.
func (c *controllerInstances) Get(name string, options metav1.GetOptions) (result *v1.ControllerInstance, err error) {
	result = &v1.ControllerInstance{}
	err = c.client.Get().
		Resource("controllerinstances").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)

	return
}

// List takes label and field selectors, and returns the list of ControllerInstances that match those selectors.
func (c *controllerInstances) List(opts metav1.ListOptions) (result *v1.ControllerInstanceList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &v1.ControllerInstanceList{}

	wgLen := 1
	// When resource version is not empty, it reads from api server local cache
	// Need to check all api server partitions
	if opts.ResourceVersion != "" && len(c.clients) > 1 {
		wgLen = len(c.clients)
	}

	if wgLen > 1 {
		var listLock sync.Mutex

		var wg sync.WaitGroup
		wg.Add(wgLen)
		results := make(map[int]*v1.ControllerInstanceList)
		errs := make(map[int]error)
		for i, client := range c.clients {
			go func(c *controllerInstances, ci rest.Interface, opts metav1.ListOptions, lock sync.Mutex, pos int, resultMap map[int]*v1.ControllerInstanceList, errMap map[int]error) {
				r := &v1.ControllerInstanceList{}
				err := ci.Get().
					Resource("controllerinstances").
					VersionedParams(&opts, scheme.ParameterCodec).
					Timeout(timeout).
					Do().
					Into(r)

				lock.Lock()
				resultMap[pos] = r
				errMap[pos] = err
				lock.Unlock()
				wg.Done()
			}(c, client, opts, listLock, i, results, errs)
		}
		wg.Wait()

		// consolidate list result
		itemsMap := make(map[string]*v1.ControllerInstance)
		for j := 0; j < wgLen; j++ {
			currentErr, isOK := errs[j]
			if isOK && currentErr != nil {
				if !(errors.IsForbidden(currentErr) && strings.Contains(currentErr.Error(), "no relationship found between node")) {
					err = currentErr
					return
				} else {
					continue
				}
			}

			currentResult, _ := results[j]
			if result.Kind == "" {
				result.TypeMeta = currentResult.TypeMeta
				result.ListMeta = currentResult.ListMeta
			}
			for _, item := range currentResult.Items {
				if _, exist := itemsMap[item.ResourceVersion]; !exist {
					itemsMap[item.ResourceVersion] = &item
				}
			}
		}

		for _, item := range itemsMap {
			result.Items = append(result.Items, *item)
		}
		return
	}

	err = c.client.Get().
		Resource("controllerinstances").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do().
		Into(result)
	if err == nil {
		return
	}

	if !(errors.IsForbidden(err) && strings.Contains(err.Error(), "no relationship found between node")) {
		return
	}

	// Found api server that works with this list, keep the client
	for _, client := range c.clients {
		if client == c.client {
			continue
		}

		err = client.Get().
			Resource("controllerinstances").
			VersionedParams(&opts, scheme.ParameterCodec).
			Timeout(timeout).
			Do().
			Into(result)

		if err == nil {
			c.client = client
			return
		}

		if err != nil && errors.IsForbidden(err) &&
			strings.Contains(err.Error(), "no relationship found between node") {
			klog.V(6).Infof("Skip error %v in list", err)
			continue
		}
	}

	return
}

// Watch returns a watch.Interface that watches the requested controllerInstances.
func (c *controllerInstances) Watch(opts metav1.ListOptions) watch.AggregatedWatchInterface {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	aggWatch := watch.NewAggregatedWatcher()
	for _, client := range c.clients {
		watcher, err := client.Get().
			Resource("controllerinstances").
			VersionedParams(&opts, scheme.ParameterCodec).
			Timeout(timeout).
			Watch()
		if err != nil && opts.AllowPartialWatch && errors.IsForbidden(err) {
			// watch error was not returned properly in error message. Skip when partial watch is allowed
			klog.V(6).Infof("Watch error for partial watch %v. options [%+v]", err, opts)
			continue
		}
		aggWatch.AddWatchInterface(watcher, err)
	}
	return aggWatch
}

// Create takes the representation of a controllerInstance and creates it.  Returns the server's representation of the controllerInstance, and an error, if there is any.
func (c *controllerInstances) Create(controllerInstance *v1.ControllerInstance) (result *v1.ControllerInstance, err error) {
	result = &v1.ControllerInstance{}

	err = c.client.Post().
		Resource("controllerinstances").
		Body(controllerInstance).
		Do().
		Into(result)

	return
}

// Update takes the representation of a controllerInstance and updates it. Returns the server's representation of the controllerInstance, and an error, if there is any.
func (c *controllerInstances) Update(controllerInstance *v1.ControllerInstance) (result *v1.ControllerInstance, err error) {
	result = &v1.ControllerInstance{}

	err = c.client.Put().
		Resource("controllerinstances").
		Name(controllerInstance.Name).
		Body(controllerInstance).
		Do().
		Into(result)

	return
}

// Delete takes name of the controllerInstance and deletes it. Returns an error if one occurs.
func (c *controllerInstances) Delete(name string, options *metav1.DeleteOptions) error {
	return c.client.Delete().
		Resource("controllerinstances").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *controllerInstances) DeleteCollection(options *metav1.DeleteOptions, listOptions metav1.ListOptions) error {
	var timeout time.Duration
	if listOptions.TimeoutSeconds != nil {
		timeout = time.Duration(*listOptions.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Resource("controllerinstances").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Timeout(timeout).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched controllerInstance.
func (c *controllerInstances) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1.ControllerInstance, err error) {
	result = &v1.ControllerInstance{}
	err = c.client.Patch(pt).
		Resource("controllerinstances").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)

	return
}

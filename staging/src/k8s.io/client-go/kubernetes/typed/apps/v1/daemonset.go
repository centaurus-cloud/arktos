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

package v1

import (
	fmt "fmt"
	strings "strings"
	sync "sync"
	"time"

	v1 "k8s.io/api/apps/v1"
	errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	diff "k8s.io/apimachinery/pkg/util/diff"
	watch "k8s.io/apimachinery/pkg/watch"
	scheme "k8s.io/client-go/kubernetes/scheme"
	rest "k8s.io/client-go/rest"
	retry "k8s.io/client-go/util/retry"
	klog "k8s.io/klog"
)

// DaemonSetsGetter has a method to return a DaemonSetInterface.
// A group's client should implement this interface.
type DaemonSetsGetter interface {
	DaemonSets(namespace string) DaemonSetInterface
	DaemonSetsWithMultiTenancy(namespace string, tenant string) DaemonSetInterface
}

// DaemonSetInterface has methods to work with DaemonSet resources.
type DaemonSetInterface interface {
	Create(*v1.DaemonSet) (*v1.DaemonSet, error)
	Update(*v1.DaemonSet) (*v1.DaemonSet, error)
	UpdateStatus(*v1.DaemonSet) (*v1.DaemonSet, error)
	Delete(name string, options *metav1.DeleteOptions) error
	DeleteCollection(options *metav1.DeleteOptions, listOptions metav1.ListOptions) error
	Get(name string, options metav1.GetOptions) (*v1.DaemonSet, error)
	List(opts metav1.ListOptions) (*v1.DaemonSetList, error)
	Watch(opts metav1.ListOptions) watch.AggregatedWatchInterface
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1.DaemonSet, err error)
	DaemonSetExpansion
}

// daemonSets implements DaemonSetInterface
type daemonSets struct {
	client  rest.Interface
	clients []rest.Interface
	ns      string
	te      string
}

// newDaemonSets returns a DaemonSets
func newDaemonSets(c *AppsV1Client, namespace string) *daemonSets {
	return newDaemonSetsWithMultiTenancy(c, namespace, "system")
}

func newDaemonSetsWithMultiTenancy(c *AppsV1Client, namespace string, tenant string) *daemonSets {
	return &daemonSets{
		client:  c.RESTClient(),
		clients: c.RESTClients(),
		ns:      namespace,
		te:      tenant,
	}
}

// Get takes name of the daemonSet, and returns the corresponding daemonSet object, and an error if there is any.
func (c *daemonSets) Get(name string, options metav1.GetOptions) (result *v1.DaemonSet, err error) {
	result = &v1.DaemonSet{}
	err = c.client.Get().
		Tenant(c.te).
		Namespace(c.ns).
		Resource("daemonsets").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)

	return
}

// List takes label and field selectors, and returns the list of DaemonSets that match those selectors.
func (c *daemonSets) List(opts metav1.ListOptions) (result *v1.DaemonSetList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &v1.DaemonSetList{}

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
		results := make(map[int]*v1.DaemonSetList)
		errs := make(map[int]error)
		for i, client := range c.clients {
			go func(c *daemonSets, ci rest.Interface, opts metav1.ListOptions, lock *sync.Mutex, pos int, resultMap map[int]*v1.DaemonSetList, errMap map[int]error) {
				r := &v1.DaemonSetList{}
				err := retry.RetryOnTimeout(retry.DefaultRetry, func() error {
					return ci.Get().
						Tenant(c.te).Namespace(c.ns).
						Resource("daemonsets").
						VersionedParams(&opts, scheme.ParameterCodec).
						Timeout(timeout).
						Do().
						Into(r)
				})

				lock.Lock()
				resultMap[pos] = r
				errMap[pos] = err
				lock.Unlock()
				wg.Done()
			}(c, client, opts, &listLock, i, results, errs)
		}
		wg.Wait()

		// consolidate list result
		itemsMap := make(map[string]v1.DaemonSet)
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
			if result.ResourceVersion == "" {
				result.TypeMeta = currentResult.TypeMeta
				result.ListMeta = currentResult.ListMeta
			} else {
				isNewer, errCompare := diff.RevisionStrIsNewer(currentResult.ResourceVersion, result.ResourceVersion)
				if errCompare != nil {
					err = errors.NewInternalError(fmt.Errorf("Invalid resource version [%v]", errCompare))
					return
				} else if isNewer {
					// Since the lists are from different api servers with different partition. When used in list and watch,
					// we cannot watch from the biggest resource version. Leave it to watch for adjustment.
					result.ResourceVersion = currentResult.ResourceVersion
				}
			}
			for _, item := range currentResult.Items {
				if _, exist := itemsMap[item.ResourceVersion]; !exist {
					itemsMap[item.ResourceVersion] = item
				}
			}
		}

		for _, item := range itemsMap {
			result.Items = append(result.Items, item)
		}
		return
	}

	// The following is used for single api server partition and/or resourceVersion is empty
	// When resourceVersion is empty, objects are read from ETCD directly and will get full
	// list of data if no permission issue. The list needs to done sequential to avoid increasing
	// system load.
	err = retry.RetryOnTimeout(retry.DefaultRetry, func() error {
		return c.client.Get().
			Tenant(c.te).Namespace(c.ns).
			Resource("daemonsets").
			VersionedParams(&opts, scheme.ParameterCodec).
			Timeout(timeout).
			Do().
			Into(result)
	})
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

		err = retry.RetryOnTimeout(retry.DefaultRetry, func() error {
			return client.Get().
				Tenant(c.te).Namespace(c.ns).
				Resource("daemonsets").
				VersionedParams(&opts, scheme.ParameterCodec).
				Timeout(timeout).
				Do().
				Into(result)
		})

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

// Watch returns a watch.Interface that watches the requested daemonSets.
func (c *daemonSets) Watch(opts metav1.ListOptions) watch.AggregatedWatchInterface {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	aggWatch := watch.NewAggregatedWatcher()
	for _, client := range c.clients {
		watcher, err := retry.RetryOnNoResponse(retry.DefaultRetry, func() (watch.Interface, error) {
			return client.Get().
				Tenant(c.te).
				Namespace(c.ns).
				Resource("daemonsets").
				VersionedParams(&opts, scheme.ParameterCodec).
				Timeout(timeout).
				Watch()
		})
		if err != nil && opts.AllowPartialWatch && errors.IsForbidden(err) {
			// watch error was not returned properly in error message. Skip when partial watch is allowed
			klog.V(6).Infof("Watch error for partial watch %v. options [%+v]", err, opts)
			continue
		}
		aggWatch.AddWatchInterface(watcher, err)
	}
	return aggWatch
}

// Create takes the representation of a daemonSet and creates it.  Returns the server's representation of the daemonSet, and an error, if there is any.
func (c *daemonSets) Create(daemonSet *v1.DaemonSet) (result *v1.DaemonSet, err error) {
	result = &v1.DaemonSet{}

	objectTenant := daemonSet.ObjectMeta.Tenant
	if objectTenant == "" {
		objectTenant = c.te
	}

	err = c.client.Post().
		Tenant(objectTenant).
		Namespace(c.ns).
		Resource("daemonsets").
		Body(daemonSet).
		Do().
		Into(result)

	return
}

// Update takes the representation of a daemonSet and updates it. Returns the server's representation of the daemonSet, and an error, if there is any.
func (c *daemonSets) Update(daemonSet *v1.DaemonSet) (result *v1.DaemonSet, err error) {
	result = &v1.DaemonSet{}

	objectTenant := daemonSet.ObjectMeta.Tenant
	if objectTenant == "" {
		objectTenant = c.te
	}

	err = c.client.Put().
		Tenant(objectTenant).
		Namespace(c.ns).
		Resource("daemonsets").
		Name(daemonSet.Name).
		Body(daemonSet).
		Do().
		Into(result)

	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().

func (c *daemonSets) UpdateStatus(daemonSet *v1.DaemonSet) (result *v1.DaemonSet, err error) {
	result = &v1.DaemonSet{}

	objectTenant := daemonSet.ObjectMeta.Tenant
	if objectTenant == "" {
		objectTenant = c.te
	}

	err = c.client.Put().
		Tenant(objectTenant).
		Namespace(c.ns).
		Resource("daemonsets").
		Name(daemonSet.Name).
		SubResource("status").
		Body(daemonSet).
		Do().
		Into(result)

	return
}

// Delete takes name of the daemonSet and deletes it. Returns an error if one occurs.
func (c *daemonSets) Delete(name string, options *metav1.DeleteOptions) error {
	return c.client.Delete().
		Tenant(c.te).
		Namespace(c.ns).
		Resource("daemonsets").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *daemonSets) DeleteCollection(options *metav1.DeleteOptions, listOptions metav1.ListOptions) error {
	var timeout time.Duration
	if listOptions.TimeoutSeconds != nil {
		timeout = time.Duration(*listOptions.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Tenant(c.te).
		Namespace(c.ns).
		Resource("daemonsets").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Timeout(timeout).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched daemonSet.
func (c *daemonSets) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1.DaemonSet, err error) {
	result = &v1.DaemonSet{}
	err = c.client.Patch(pt).
		Tenant(c.te).
		Namespace(c.ns).
		Resource("daemonsets").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)

	return
}

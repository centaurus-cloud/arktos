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

package internalversion

import (
	fmt "fmt"
	strings "strings"
	sync "sync"
	"time"

	errors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	diff "k8s.io/apimachinery/pkg/util/diff"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
	klog "k8s.io/klog"
	apiregistration "k8s.io/kube-aggregator/pkg/apis/apiregistration"
	scheme "k8s.io/kube-aggregator/pkg/client/clientset_generated/internalclientset/scheme"
)

// APIServicesGetter has a method to return a APIServiceInterface.
// A group's client should implement this interface.
type APIServicesGetter interface {
	APIServices() APIServiceInterface
	APIServicesWithMultiTenancy(tenant string) APIServiceInterface
}

// APIServiceInterface has methods to work with APIService resources.
type APIServiceInterface interface {
	Create(*apiregistration.APIService) (*apiregistration.APIService, error)
	Update(*apiregistration.APIService) (*apiregistration.APIService, error)
	UpdateStatus(*apiregistration.APIService) (*apiregistration.APIService, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*apiregistration.APIService, error)
	List(opts v1.ListOptions) (*apiregistration.APIServiceList, error)
	Watch(opts v1.ListOptions) watch.AggregatedWatchInterface
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *apiregistration.APIService, err error)
	APIServiceExpansion
}

// aPIServices implements APIServiceInterface
type aPIServices struct {
	client  rest.Interface
	clients []rest.Interface
	te      string
}

// newAPIServices returns a APIServices
func newAPIServices(c *ApiregistrationClient) *aPIServices {
	return newAPIServicesWithMultiTenancy(c, "system")
}

func newAPIServicesWithMultiTenancy(c *ApiregistrationClient, tenant string) *aPIServices {
	return &aPIServices{
		client:  c.RESTClient(),
		clients: c.RESTClients(),
		te:      tenant,
	}
}

// Get takes name of the aPIService, and returns the corresponding aPIService object, and an error if there is any.
func (c *aPIServices) Get(name string, options v1.GetOptions) (result *apiregistration.APIService, err error) {
	result = &apiregistration.APIService{}
	err = c.client.Get().
		Tenant(c.te).
		Resource("apiservices").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)

	return
}

// List takes label and field selectors, and returns the list of APIServices that match those selectors.
func (c *aPIServices) List(opts v1.ListOptions) (result *apiregistration.APIServiceList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &apiregistration.APIServiceList{}

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
		results := make(map[int]*apiregistration.APIServiceList)
		errs := make(map[int]error)
		for i, client := range c.clients {
			go func(c *aPIServices, ci rest.Interface, opts v1.ListOptions, lock *sync.Mutex, pos int, resultMap map[int]*apiregistration.APIServiceList, errMap map[int]error) {
				r := &apiregistration.APIServiceList{}
				err := ci.Get().
					Tenant(c.te).
					Resource("apiservices").
					VersionedParams(&opts, scheme.ParameterCodec).
					Timeout(timeout).
					Do().
					Into(r)

				lock.Lock()
				resultMap[pos] = r
				errMap[pos] = err
				lock.Unlock()
				wg.Done()
			}(c, client, opts, &listLock, i, results, errs)
		}
		wg.Wait()

		// consolidate list result
		itemsMap := make(map[string]apiregistration.APIService)
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
	err = c.client.Get().
		Tenant(c.te).
		Resource("apiservices").
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
			Tenant(c.te).
			Resource("apiservices").
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

// Watch returns a watch.Interface that watches the requested aPIServices.
func (c *aPIServices) Watch(opts v1.ListOptions) watch.AggregatedWatchInterface {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	aggWatch := watch.NewAggregatedWatcher()
	for _, client := range c.clients {
		watcher, err := client.Get().
			Tenant(c.te).
			Resource("apiservices").
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

// Create takes the representation of a aPIService and creates it.  Returns the server's representation of the aPIService, and an error, if there is any.
func (c *aPIServices) Create(aPIService *apiregistration.APIService) (result *apiregistration.APIService, err error) {
	result = &apiregistration.APIService{}

	objectTenant := aPIService.ObjectMeta.Tenant
	if objectTenant == "" {
		objectTenant = c.te
	}

	err = c.client.Post().
		Tenant(objectTenant).
		Resource("apiservices").
		Body(aPIService).
		Do().
		Into(result)

	return
}

// Update takes the representation of a aPIService and updates it. Returns the server's representation of the aPIService, and an error, if there is any.
func (c *aPIServices) Update(aPIService *apiregistration.APIService) (result *apiregistration.APIService, err error) {
	result = &apiregistration.APIService{}

	objectTenant := aPIService.ObjectMeta.Tenant
	if objectTenant == "" {
		objectTenant = c.te
	}

	err = c.client.Put().
		Tenant(objectTenant).
		Resource("apiservices").
		Name(aPIService.Name).
		Body(aPIService).
		Do().
		Into(result)

	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().

func (c *aPIServices) UpdateStatus(aPIService *apiregistration.APIService) (result *apiregistration.APIService, err error) {
	result = &apiregistration.APIService{}

	objectTenant := aPIService.ObjectMeta.Tenant
	if objectTenant == "" {
		objectTenant = c.te
	}

	err = c.client.Put().
		Tenant(objectTenant).
		Resource("apiservices").
		Name(aPIService.Name).
		SubResource("status").
		Body(aPIService).
		Do().
		Into(result)

	return
}

// Delete takes name of the aPIService and deletes it. Returns an error if one occurs.
func (c *aPIServices) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Tenant(c.te).
		Resource("apiservices").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *aPIServices) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	var timeout time.Duration
	if listOptions.TimeoutSeconds != nil {
		timeout = time.Duration(*listOptions.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Tenant(c.te).
		Resource("apiservices").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Timeout(timeout).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched aPIService.
func (c *aPIServices) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *apiregistration.APIService, err error) {
	result = &apiregistration.APIService{}
	err = c.client.Patch(pt).
		Tenant(c.te).
		Resource("apiservices").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)

	return
}

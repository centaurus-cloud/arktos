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

package v1alpha1

import (
	fmt "fmt"
	strings "strings"
	sync "sync"
	"time"

	errors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	diff "k8s.io/apimachinery/pkg/util/diff"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
	klog "k8s.io/klog"
	v1alpha1 "k8s.io/metrics/pkg/apis/metrics/v1alpha1"
	scheme "k8s.io/metrics/pkg/client/clientset/versioned/scheme"
)

// PodMetricsesGetter has a method to return a PodMetricsInterface.
// A group's client should implement this interface.
type PodMetricsesGetter interface {
	PodMetricses(namespace string) PodMetricsInterface
	PodMetricsesWithMultiTenancy(namespace string, tenant string) PodMetricsInterface
}

// PodMetricsInterface has methods to work with PodMetrics resources.
type PodMetricsInterface interface {
	Get(name string, options v1.GetOptions) (*v1alpha1.PodMetrics, error)
	List(opts v1.ListOptions) (*v1alpha1.PodMetricsList, error)
	Watch(opts v1.ListOptions) watch.AggregatedWatchInterface
	PodMetricsExpansion
}

// podMetricses implements PodMetricsInterface
type podMetricses struct {
	client  rest.Interface
	clients []rest.Interface
	ns      string
	te      string
}

// newPodMetricses returns a PodMetricses
func newPodMetricses(c *MetricsV1alpha1Client, namespace string) *podMetricses {
	return newPodMetricsesWithMultiTenancy(c, namespace, "system")
}

func newPodMetricsesWithMultiTenancy(c *MetricsV1alpha1Client, namespace string, tenant string) *podMetricses {
	return &podMetricses{
		client:  c.RESTClient(),
		clients: c.RESTClients(),
		ns:      namespace,
		te:      tenant,
	}
}

// Get takes name of the podMetrics, and returns the corresponding podMetrics object, and an error if there is any.
func (c *podMetricses) Get(name string, options v1.GetOptions) (result *v1alpha1.PodMetrics, err error) {
	result = &v1alpha1.PodMetrics{}
	err = c.client.Get().
		Tenant(c.te).
		Namespace(c.ns).
		Resource("pods").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)

	return
}

// List takes label and field selectors, and returns the list of PodMetricses that match those selectors.
func (c *podMetricses) List(opts v1.ListOptions) (result *v1alpha1.PodMetricsList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &v1alpha1.PodMetricsList{}

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
		results := make(map[int]*v1alpha1.PodMetricsList)
		errs := make(map[int]error)
		for i, client := range c.clients {
			go func(c *podMetricses, ci rest.Interface, opts v1.ListOptions, lock sync.Mutex, pos int, resultMap map[int]*v1alpha1.PodMetricsList, errMap map[int]error) {
				r := &v1alpha1.PodMetricsList{}
				err := ci.Get().
					Tenant(c.te).Namespace(c.ns).
					Resource("pods").
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
		itemsMap := make(map[string]*v1alpha1.PodMetrics)
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
					itemsMap[item.ResourceVersion] = &item
				}
			}
		}

		for _, item := range itemsMap {
			result.Items = append(result.Items, *item)
		}
		return
	}

	// The following is used for single api server partition and/or resourceVersion is empty
	// When resourceVersion is empty, objects are read from ETCD directly and will get full
	// list of data if no permission issue. The list needs to done sequential to avoid increasing
	// system load.
	err = c.client.Get().
		Tenant(c.te).Namespace(c.ns).
		Resource("pods").
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
			Tenant(c.te).Namespace(c.ns).
			Resource("pods").
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

// Watch returns a watch.Interface that watches the requested podMetricses.
func (c *podMetricses) Watch(opts v1.ListOptions) watch.AggregatedWatchInterface {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	aggWatch := watch.NewAggregatedWatcher()
	for _, client := range c.clients {
		watcher, err := client.Get().
			Tenant(c.te).
			Namespace(c.ns).
			Resource("pods").
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

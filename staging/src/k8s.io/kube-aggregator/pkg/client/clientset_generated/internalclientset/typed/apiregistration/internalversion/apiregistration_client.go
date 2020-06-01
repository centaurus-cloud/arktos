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
	"time"

	rand "k8s.io/apimachinery/pkg/util/rand"
	apiserverupdate "k8s.io/client-go/apiserverupdate"
	rest "k8s.io/client-go/rest"
	klog "k8s.io/klog"
	"k8s.io/kube-aggregator/pkg/client/clientset_generated/internalclientset/scheme"
)

type ApiregistrationInterface interface {
	RESTClient() rest.Interface
	RESTClients() []rest.Interface
	APIServicesGetter
}

// ApiregistrationClient is used to interact with features provided by the apiregistration.k8s.io group.
type ApiregistrationClient struct {
	restClients []rest.Interface
	configs     *rest.Config
}

func (c *ApiregistrationClient) APIServices() APIServiceInterface {
	return newAPIServicesWithMultiTenancy(c, "system")
}

func (c *ApiregistrationClient) APIServicesWithMultiTenancy(tenant string) APIServiceInterface {
	return newAPIServicesWithMultiTenancy(c, tenant)
}

// NewForConfig creates a new ApiregistrationClient for the given config.
func NewForConfig(c *rest.Config) (*ApiregistrationClient, error) {
	configs := rest.CopyConfigs(c)
	if err := setConfigDefaults(configs); err != nil {
		return nil, err
	}

	clients := make([]rest.Interface, len(configs.GetAllConfigs()))
	for i, config := range configs.GetAllConfigs() {
		client, err := rest.RESTClientFor(config)
		if err != nil {
			return nil, err
		}
		clients[i] = client
	}

	obj := &ApiregistrationClient{
		restClients: clients,
		configs:     configs,
	}

	obj.run()

	return obj, nil
}

// NewForConfigOrDie creates a new ApiregistrationClient for the given config and
// panics if there is an error in the config.
func NewForConfigOrDie(c *rest.Config) *ApiregistrationClient {
	client, err := NewForConfig(c)
	if err != nil {
		panic(err)
	}
	return client
}

// New creates a new ApiregistrationClient for the given RESTClient.
func New(c rest.Interface) *ApiregistrationClient {
	clients := []rest.Interface{c}
	return &ApiregistrationClient{restClients: clients}
}

func setConfigDefaults(configs *rest.Config) error {
	for _, config := range configs.GetAllConfigs() {
		config.APIPath = "/apis"
		if config.UserAgent == "" {
			config.UserAgent = rest.DefaultKubernetesUserAgent()
		}
		if config.GroupVersion == nil || config.GroupVersion.Group != scheme.Scheme.PrioritizedVersionsForGroup("apiregistration.k8s.io")[0].Group {
			gv := scheme.Scheme.PrioritizedVersionsForGroup("apiregistration.k8s.io")[0]
			config.GroupVersion = &gv
		}
		config.NegotiatedSerializer = scheme.Codecs

		if config.QPS == 0 {
			config.QPS = 5
		}
		if config.Burst == 0 {
			config.Burst = 10
		}
	}

	return nil
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *ApiregistrationClient) RESTClient() rest.Interface {
	if c == nil {
		return nil
	}

	max := len(c.restClients)
	if max == 0 {
		return nil
	}
	if max == 1 {
		return c.restClients[0]
	}

	rand.Seed(time.Now().UnixNano())
	ran := rand.IntnRange(0, max-1)
	return c.restClients[ran]
}

// RESTClients returns all RESTClient that are used to communicate
// with all API servers by this client implementation.
func (c *ApiregistrationClient) RESTClients() []rest.Interface {
	if c == nil {
		return nil
	}

	return c.restClients
}

// run watch api server instance updates and recreate connections to new set of api servers
func (c *ApiregistrationClient) run() {
	go func(c *ApiregistrationClient) {
		member := c.configs.WatchUpdate()
		watcherForUpdateComplete := apiserverupdate.GetClientSetsWatcher()
		watcherForUpdateComplete.AddWatcher()

		for range member.Read {
			// create new client
			clients := make([]rest.Interface, len(c.configs.GetAllConfigs()))
			for i, config := range c.configs.GetAllConfigs() {
				client, err := rest.RESTClientFor(config)
				if err != nil {
					klog.Fatalf("Cannot create rest client for [%+v], err %v", config, err)
					return
				}
				clients[i] = client
			}
			c.restClients = clients
			watcherForUpdateComplete.NotifyDone()
		}
	}(c)
}

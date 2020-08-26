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
	rand "math/rand"
	"sync"
	"time"

	apiserverupdate "k8s.io/client-go/apiserverupdate"
	rest "k8s.io/client-go/rest"
	v1 "k8s.io/code-generator/_examples/apiserver/apis/example2/v1"
	"k8s.io/code-generator/_examples/apiserver/clientset/versioned/scheme"
	klog "k8s.io/klog"
)

type SecondExampleV1Interface interface {
	RESTClient() rest.Interface
	RESTClients() []rest.Interface
	TestTypesGetter
}

// SecondExampleV1Client is used to interact with features provided by the example.test.apiserver.code-generator.k8s.io group.
type SecondExampleV1Client struct {
	restClients []rest.Interface
	configs     *rest.Config
	mux         sync.RWMutex
}

func (c *SecondExampleV1Client) TestTypes(namespace string) TestTypeInterface {
	return newTestTypesWithMultiTenancy(c, namespace, "system")
}

func (c *SecondExampleV1Client) TestTypesWithMultiTenancy(namespace string, tenant string) TestTypeInterface {
	return newTestTypesWithMultiTenancy(c, namespace, tenant)
}

// NewForConfig creates a new SecondExampleV1Client for the given config.
func NewForConfig(c *rest.Config) (*SecondExampleV1Client, error) {
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

	obj := &SecondExampleV1Client{
		restClients: clients,
		configs:     configs,
	}

	obj.run()

	return obj, nil
}

// NewForConfigOrDie creates a new SecondExampleV1Client for the given config and
// panics if there is an error in the config.
func NewForConfigOrDie(c *rest.Config) *SecondExampleV1Client {
	client, err := NewForConfig(c)
	if err != nil {
		panic(err)
	}
	return client
}

// New creates a new SecondExampleV1Client for the given RESTClient.
func New(c rest.Interface) *SecondExampleV1Client {
	clients := []rest.Interface{c}
	return &SecondExampleV1Client{restClients: clients}
}

func setConfigDefaults(configs *rest.Config) error {
	gv := v1.SchemeGroupVersion

	for _, config := range configs.GetAllConfigs() {
		config.GroupVersion = &gv
		config.APIPath = "/apis"
		config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()

		if config.UserAgent == "" {
			config.UserAgent = rest.DefaultKubernetesUserAgent()
		}
	}

	return nil
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *SecondExampleV1Client) RESTClient() rest.Interface {
	if c == nil {
		return nil
	}

	c.mux.RLock()
	defer c.mux.RUnlock()
	max := len(c.restClients)
	if max == 0 {
		return nil
	}
	if max == 1 {
		return c.restClients[0]
	}

	rand.Seed(time.Now().UnixNano())
	ran := rand.Intn(max)
	return c.restClients[ran]
}

// RESTClients returns all RESTClient that are used to communicate
// with all API servers by this client implementation.
func (c *SecondExampleV1Client) RESTClients() []rest.Interface {
	if c == nil {
		return nil
	}
	c.mux.RLock()
	defer c.mux.RUnlock()
	return c.restClients
}

// run watch api server instance updates and recreate connections to new set of api servers
func (c *SecondExampleV1Client) run() {
	go func(c *SecondExampleV1Client) {
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
			c.mux.Lock()
			klog.Infof("Reset restClients. length %v -> %v", len(c.restClients), len(clients))
			c.restClients = clients
			c.mux.Unlock()
			watcherForUpdateComplete.NotifyDone()
		}
	}(c)
}

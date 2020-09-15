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
	v1beta1 "k8s.io/client-go/kubernetes/typed/policy/v1beta1"
	rest "k8s.io/client-go/rest"
	testing "k8s.io/client-go/testing"
)

type FakePolicyV1beta1 struct {
	*testing.Fake
}

func (c *FakePolicyV1beta1) Evictions(namespace string) v1beta1.EvictionInterface {
	return &FakeEvictions{c, namespace, "system"}
}

func (c *FakePolicyV1beta1) EvictionsWithMultiTenancy(namespace string, tenant string) v1beta1.EvictionInterface {
	return &FakeEvictions{c, namespace, tenant}
}

func (c *FakePolicyV1beta1) PodDisruptionBudgets(namespace string) v1beta1.PodDisruptionBudgetInterface {
	return &FakePodDisruptionBudgets{c, namespace, "system"}
}

func (c *FakePolicyV1beta1) PodDisruptionBudgetsWithMultiTenancy(namespace string, tenant string) v1beta1.PodDisruptionBudgetInterface {
	return &FakePodDisruptionBudgets{c, namespace, tenant}
}

func (c *FakePolicyV1beta1) PodSecurityPolicies() v1beta1.PodSecurityPolicyInterface {
	return &FakePodSecurityPolicies{c, "system"}
}

func (c *FakePolicyV1beta1) PodSecurityPoliciesWithMultiTenancy(tenant string) v1beta1.PodSecurityPolicyInterface {
	return &FakePodSecurityPolicies{c, tenant}
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *FakePolicyV1beta1) RESTClient() rest.Interface {
	var ret *rest.RESTClient
	return ret
}

// RESTClients returns all RESTClient that are used to communicate
// with all API servers by this client implementation.
func (c *FakePolicyV1beta1) RESTClients() []rest.Interface {
	var ret *rest.RESTClient
	return []rest.Interface{ret}
}

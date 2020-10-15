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
	v1 "k8s.io/client-go/kubernetes/typed/storage/v1"
	rest "k8s.io/client-go/rest"
	testing "k8s.io/client-go/testing"
)

type FakeStorageV1 struct {
	*testing.Fake
}

func (c *FakeStorageV1) StorageClasses() v1.StorageClassInterface {
	return &FakeStorageClasses{c, "system"}
}

func (c *FakeStorageV1) StorageClassesWithMultiTenancy(tenant string) v1.StorageClassInterface {
	return &FakeStorageClasses{c, tenant}
}

func (c *FakeStorageV1) VolumeAttachments() v1.VolumeAttachmentInterface {
	return &FakeVolumeAttachments{c, "system"}
}

func (c *FakeStorageV1) VolumeAttachmentsWithMultiTenancy(tenant string) v1.VolumeAttachmentInterface {
	return &FakeVolumeAttachments{c, tenant}
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *FakeStorageV1) RESTClient() rest.Interface {
	var ret *rest.RESTClient
	return ret
}

// RESTClients returns all RESTClient that are used to communicate
// with all API servers by this client implementation.
func (c *FakeStorageV1) RESTClients() []rest.Interface {
	var ret *rest.RESTClient
	return []rest.Interface{ret}
}
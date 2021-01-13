/*
Copyright 2018 The Kubernetes Authors.
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

package reference

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type TestRuntimeObj struct {
	metav1.TypeMeta
	metav1.ObjectMeta
}

func (o *TestRuntimeObj) DeepCopyObject() runtime.Object {
	panic("die")
}

func TestGetReferenceRefVersion(t *testing.T) {
	tests := []struct {
		name               string
		input              *TestRuntimeObj
		groupVersion       schema.GroupVersion
		expectedRefVersion string
	}{
		{
			name: "v1 GV from scheme",
			input: &TestRuntimeObj{
				ObjectMeta: metav1.ObjectMeta{SelfLink: "/bad-selflink/unused"},
			},
			groupVersion:       schema.GroupVersion{Group: "", Version: "v1"},
			expectedRefVersion: "v1",
		},
		{
			name: "foo.group/v3 GV from scheme",
			input: &TestRuntimeObj{
				ObjectMeta: metav1.ObjectMeta{SelfLink: "/bad-selflink/unused"},
			},
			groupVersion:       schema.GroupVersion{Group: "foo.group", Version: "v3"},
			expectedRefVersion: "foo.group/v3",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			scheme.AddKnownTypes(test.groupVersion, &TestRuntimeObj{})
			ref, err := GetReference(scheme, test.input)
			if err != nil {
				t.Fatal(err)
			}
			if test.expectedRefVersion != ref.APIVersion {
				t.Errorf("expected %q, got %q", test.expectedRefVersion, ref.APIVersion)
			}
		})
	}
}

func TestGetReferenceObjectMeta(t *testing.T) {
	tests := []struct {
		name              string
		input             *TestRuntimeObj
		expectedTenant    string
		expectedNamespace string
	}{
		{
			name: "tenant from ObjectMeta",
			input: &TestRuntimeObj{
				ObjectMeta: metav1.ObjectMeta{Tenant: "test-te", Namespace: "test-ns", Name: "pod1", SelfLink: "/api/v1/tenatns/test-te/namespaces/test-ns/pods/pod1"},
			},
			expectedTenant:    "test-te",
			expectedNamespace: "test-ns",
		},
	}

	scheme := runtime.NewScheme()
	scheme.AddKnownTypes(schema.GroupVersion{Group: "this", Version: "is ignored"}, &TestRuntimeObj{})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ref, err := GetReference(scheme, test.input)
			if err != nil {
				t.Fatal(err)
			}
			if test.expectedTenant != ref.Tenant {
				t.Errorf("expected tenant %q, got %q", test.expectedTenant, ref.Tenant)
			}
			if test.expectedNamespace != ref.Namespace {
				t.Errorf("expected namespace %q, got %q", test.expectedNamespace, ref.Namespace)
			}
		})
	}
}

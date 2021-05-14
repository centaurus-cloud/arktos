/*
Copyright 2017 The Kubernetes Authors.
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

package rest

import (
	"context"
	uid "github.com/google/uuid"
	"k8s.io/apimachinery/pkg/apis/meta/fuzzer"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/klog"
)

// FillObjectMetaSystemFields populates fields that are managed by the system on ObjectMeta.
func FillObjectMetaSystemFields(meta metav1.Object) {
	meta.SetCreationTimestamp(metav1.Now())
	uidStr := string(meta.GetUID())
	if len(uidStr) == 0 {
		uid := uuid.NewUUID()
		meta.SetUID(uid)
	} else {
		_, err := uid.Parse(uidStr)
		if err != nil {
			uid := uuid.NewUUID()
			meta.SetUID(uid)
			klog.V(3).Infof("Got invalid uuid [%s]. Assign a new one [%v]", uidStr, uid)
		}
	}
	uid := meta.GetUID()
	meta.SetHashKey(fuzzer.GetHashOfUUID(uid))
	meta.SetSelfLink("")
}

// ValidNamespace returns false if the namespace on the context differs from
// the resource.  If the resource has no namespace, it is set to the value in
// the context.
func ValidNamespace(ctx context.Context, resource metav1.Object) bool {
	ns, ok := genericapirequest.NamespaceFrom(ctx)
	if len(resource.GetNamespace()) == 0 {
		resource.SetNamespace(ns)
	}
	return ns == resource.GetNamespace() && ok
}

// ValidTenant returns false if the tenant on the context differs from
// the resource.  If the resource has no tenant, it is set to the value in
// the context.
func ValidTenant(ctx context.Context, resource metav1.Object) bool {
	tenant, ok := genericapirequest.TenantFrom(ctx)

	// When we are here, we know that the resource need a valid tenant value.
	// If the tenant value is empty, we set it to the default one, "system".
	if tenant == "" {
		tenant = metav1.TenantSystem
	}

	if len(resource.GetTenant()) == 0 {
		resource.SetTenant(tenant)
	}
	return tenant == resource.GetTenant() && ok
}

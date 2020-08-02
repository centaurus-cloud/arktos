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

package apiserver

import (
	"fmt"
	"sort"
	"time"

	"k8s.io/klog"

	autoscaling "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/apiserver/pkg/endpoints/discovery"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	informers "k8s.io/apiextensions-apiserver/pkg/client/informers/internalversion/apiextensions/internalversion"
	listers "k8s.io/apiextensions-apiserver/pkg/client/listers/apiextensions/internalversion"
	crdregistry "k8s.io/apiextensions-apiserver/pkg/registry/customresourcedefinition"
)

type DiscoveryController struct {
	versionHandler *versionDiscoveryHandler
	groupHandler   *groupDiscoveryHandler

	crdLister  listers.CustomResourceDefinitionLister
	crdsSynced cache.InformerSynced

	// To allow injection for testing.
	syncFn func(gvt GroupVersionTenant) error

	queue workqueue.RateLimitingInterface
}

type GroupVersionTenant struct {
	group   string
	version string
	tenant  string
}

func NewDiscoveryController(crdInformer informers.CustomResourceDefinitionInformer, versionHandler *versionDiscoveryHandler, groupHandler *groupDiscoveryHandler) *DiscoveryController {
	c := &DiscoveryController{
		versionHandler: versionHandler,
		groupHandler:   groupHandler,
		crdLister:      crdInformer.Lister(),
		crdsSynced:     crdInformer.Informer().HasSynced,

		queue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "DiscoveryController"),
	}

	crdInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addCustomResourceDefinition,
		UpdateFunc: c.updateCustomResourceDefinition,
		DeleteFunc: c.deleteCustomResourceDefinition,
	})

	c.syncFn = c.sync

	return c
}

func (c *DiscoveryController) sync(gvt GroupVersionTenant) error {

	apiVersionsForDiscovery := []metav1.GroupVersionForDiscovery{}
	systemSharedApiVersionsForDiscovery := []metav1.GroupVersionForDiscovery{}
	apiResourcesForDiscovery := []metav1.APIResource{}
	systemSharedCrdForDiscovery := []metav1.APIResource{}
	versionsForDiscoveryMap := map[metav1.GroupVersion]bool{}
	systemSharedVersionsForDiscoveryMap := map[metav1.GroupVersion]bool{}

	groupVersion := schema.GroupVersion{Group: gvt.group, Version: gvt.version}

	crds, err := c.crdLister.CustomResourceDefinitionsWithMultiTenancy(gvt.tenant).List(labels.Everything())
	if err != nil {
		return err
	}
	foundVersion := false
	foundGroup := false

	foundSystemSharedCrd := false
	for _, crd := range crds {
		if !apiextensions.IsCRDConditionTrue(crd, apiextensions.Established) {
			continue
		}

		if crd.Spec.Group != gvt.group {
			continue
		}

		foundThisVersion := false
		isSystemSharedCrd := crdregistry.IsCrdSystemForced(crd)
		if isSystemSharedCrd {
			foundSystemSharedCrd = true
		}
		var storageVersionHash string
		var apiResource metav1.APIResource
		for _, v := range crd.Spec.Versions {
			if !v.Served {
				continue
			}

			// If there is any Served version, that means the group should show up in discovery
			foundGroup = true

			gv := metav1.GroupVersion{Group: crd.Spec.Group, Version: v.Name}
			gvForDiscovery := metav1.GroupVersionForDiscovery{
				GroupVersion: crd.Spec.Group + "/" + v.Name,
				Version:      v.Name,
			}

			if !versionsForDiscoveryMap[gv] {
				versionsForDiscoveryMap[gv] = true
				apiVersionsForDiscovery = append(apiVersionsForDiscovery, gvForDiscovery)
			}

			if isSystemSharedCrd && !systemSharedVersionsForDiscoveryMap[gv] {
				systemSharedVersionsForDiscoveryMap[gv] = true
				systemSharedApiVersionsForDiscovery = append(systemSharedApiVersionsForDiscovery, gvForDiscovery)
			}

			if v.Name == gvt.version {
				foundThisVersion = true
			}
			if v.Storage {
				storageVersionHash = discovery.StorageVersionHash(gv.Group, gv.Version, crd.Spec.Names.Kind)
			}
		}

		if !foundThisVersion {
			continue
		}
		foundVersion = true

		verbs := metav1.Verbs([]string{"delete", "deletecollection", "get", "list", "patch", "create", "update", "watch"})
		// if we're terminating we don't allow some verbs
		if apiextensions.IsCRDConditionTrue(crd, apiextensions.Terminating) {
			verbs = metav1.Verbs([]string{"delete", "deletecollection", "get", "list", "watch"})
		}

		apiResource = metav1.APIResource{
			Name:               crd.Status.AcceptedNames.Plural,
			SingularName:       crd.Status.AcceptedNames.Singular,
			Namespaced:         crd.Spec.Scope == apiextensions.NamespaceScoped,
			Tenanted:           crd.Spec.Scope == apiextensions.NamespaceScoped || crd.Spec.Scope == apiextensions.TenantScoped,
			Kind:               crd.Status.AcceptedNames.Kind,
			Verbs:              verbs,
			ShortNames:         crd.Status.AcceptedNames.ShortNames,
			Categories:         crd.Status.AcceptedNames.Categories,
			StorageVersionHash: storageVersionHash,
		}

		apiResourcesForDiscovery = append(apiResourcesForDiscovery, apiResource)
		if isSystemSharedCrd {
			systemSharedCrdForDiscovery = append(apiResourcesForDiscovery, apiResource)
		}

		subresources, err := apiextensions.GetSubresourcesForVersion(crd, gvt.version)
		if err != nil {
			return err
		}
		if subresources != nil && subresources.Status != nil {
			apiResource = metav1.APIResource{
				Name:       crd.Status.AcceptedNames.Plural + "/status",
				Namespaced: crd.Spec.Scope == apiextensions.NamespaceScoped,
				Tenanted:   crd.Spec.Scope == apiextensions.NamespaceScoped || crd.Spec.Scope == apiextensions.TenantScoped,
				Kind:       crd.Status.AcceptedNames.Kind,
				Verbs:      metav1.Verbs([]string{"get", "patch", "update"}),
			}

			apiResourcesForDiscovery = append(apiResourcesForDiscovery, apiResource)

			if isSystemSharedCrd {
				systemSharedCrdForDiscovery = append(apiResourcesForDiscovery, apiResource)
			}
		}

		if subresources != nil && subresources.Scale != nil {
			apiResource = metav1.APIResource{
				Group:      autoscaling.GroupName,
				Version:    "v1",
				Kind:       "Scale",
				Name:       crd.Status.AcceptedNames.Plural + "/scale",
				Namespaced: crd.Spec.Scope == apiextensions.NamespaceScoped,
				Tenanted:   crd.Spec.Scope == apiextensions.NamespaceScoped || crd.Spec.Scope == apiextensions.TenantScoped,
				Verbs:      metav1.Verbs([]string{"get", "patch", "update"}),
			}

			apiResourcesForDiscovery = append(apiResourcesForDiscovery, apiResource)

			if isSystemSharedCrd {
				systemSharedCrdForDiscovery = append(apiResourcesForDiscovery, apiResource)
			}
		}
	}

	if !foundGroup {
		c.groupHandler.unsetDiscovery(gvt.tenant, gvt.group)
		c.versionHandler.unsetDiscovery(gvt.tenant, groupVersion)
		if gvt.tenant == metav1.TenantSystem {
			c.groupHandler.unsetSystemSharedCrdDiscovery(gvt.group)
			c.versionHandler.unsetSystemSharedCrdDiscovery(groupVersion)
		}
		return nil
	}

	sortGroupDiscoveryByKubeAwareVersion(apiVersionsForDiscovery)
	sortGroupDiscoveryByKubeAwareVersion(systemSharedApiVersionsForDiscovery)

	apiGroup := metav1.APIGroup{
		Name:     gvt.group,
		Versions: apiVersionsForDiscovery,
		// the preferred versions for a group is the first item in
		// apiVersionsForDiscovery after it put in the right ordered
		PreferredVersion: apiVersionsForDiscovery[0],
	}
	c.groupHandler.setDiscovery(gvt.tenant, gvt.group, discovery.NewAPIGroupHandler(Codecs, apiGroup))

	if foundSystemSharedCrd {
		systemSharedAPiGroup := metav1.APIGroup{
			Name:     gvt.group,
			Versions: systemSharedApiVersionsForDiscovery,
			// apiVersionsForDiscovery after it put in the right ordered
			PreferredVersion: systemSharedApiVersionsForDiscovery[0],
		}
		c.groupHandler.setSystemSharedCrdDiscovery(gvt.group, discovery.NewAPIGroupHandler(Codecs, systemSharedAPiGroup))
	}

	if !foundVersion {
		c.versionHandler.unsetDiscovery(gvt.tenant, groupVersion)
		if gvt.tenant == metav1.TenantSystem {
			c.versionHandler.unsetSystemSharedCrdDiscovery(groupVersion)
		}
		return nil
	}

	c.versionHandler.setDiscovery(gvt.tenant, groupVersion, discovery.NewAPIVersionHandler(Codecs, groupVersion, discovery.APIResourceListerFunc(func() []metav1.APIResource {
		return apiResourcesForDiscovery
	})))

	if gvt.tenant == metav1.TenantSystem {
		c.versionHandler.setSystemSharedCrdDiscovery(groupVersion, discovery.NewAPIVersionHandler(Codecs, groupVersion, discovery.APIResourceListerFunc(func() []metav1.APIResource {
			return systemSharedCrdForDiscovery
		})))
	}

	return nil
}

func sortGroupDiscoveryByKubeAwareVersion(gd []metav1.GroupVersionForDiscovery) {
	sort.Slice(gd, func(i, j int) bool {
		return version.CompareKubeAwareVersionStrings(gd[i].Version, gd[j].Version) > 0
	})
}

func (c *DiscoveryController) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()
	defer klog.Infof("Shutting down DiscoveryController")

	klog.Infof("Starting DiscoveryController")

	if !cache.WaitForCacheSync(stopCh, c.crdsSynced) {
		utilruntime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
		return
	}

	// only start one worker thread since its a slow moving API
	go wait.Until(c.runWorker, time.Second, stopCh)

	<-stopCh
}

func (c *DiscoveryController) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem deals with one key off the queue.  It returns false when it's time to quit.
func (c *DiscoveryController) processNextWorkItem() bool {
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(key)

	err := c.syncFn(key.(GroupVersionTenant))
	if err == nil {
		c.queue.Forget(key)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("%v failed with: %v", key, err))
	c.queue.AddRateLimited(key)

	return true
}

func (c *DiscoveryController) enqueue(obj *apiextensions.CustomResourceDefinition) {
	for _, v := range obj.Spec.Versions {
		c.queue.Add(GroupVersionTenant{obj.Spec.Group, v.Name, obj.Tenant})
	}
}

func (c *DiscoveryController) addCustomResourceDefinition(obj interface{}) {
	castObj := obj.(*apiextensions.CustomResourceDefinition)
	klog.V(4).Infof("Adding customresourcedefinition %s", castObj.Name)
	c.enqueue(castObj)
}

func (c *DiscoveryController) updateCustomResourceDefinition(oldObj, newObj interface{}) {
	castNewObj := newObj.(*apiextensions.CustomResourceDefinition)
	castOldObj := oldObj.(*apiextensions.CustomResourceDefinition)
	klog.V(4).Infof("Updating customresourcedefinition %s", castOldObj.Name)
	// Enqueue both old and new object to make sure we remove and add appropriate Versions.
	// The working queue will resolve any duplicates and only changes will stay in the queue.
	c.enqueue(castNewObj)
	c.enqueue(castOldObj)
}

func (c *DiscoveryController) deleteCustomResourceDefinition(obj interface{}) {
	castObj, ok := obj.(*apiextensions.CustomResourceDefinition)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			klog.Errorf("Couldn't get object from tombstone %#v", obj)
			return
		}
		castObj, ok = tombstone.Obj.(*apiextensions.CustomResourceDefinition)
		if !ok {
			klog.Errorf("Tombstone contained object that is not expected %#v", obj)
			return
		}
	}
	klog.V(4).Infof("Deleting customresourcedefinition %q", castObj.Name)
	c.enqueue(castObj)
}

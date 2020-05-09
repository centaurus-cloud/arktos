/*
Copyright 2020 Authors of Arktos.

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

package tenant

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/util/json"
	"text/template"
	"time"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	arktosv1 "k8s.io/arktos-ext/pkg/apis/arktosextensions/v1"
	arktos "k8s.io/arktos-ext/pkg/generated/clientset/versioned"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/util/metrics"

	"k8s.io/klog"
)

var tenant_default_namespaces = [...]string{core.NamespaceDefault, core.NamespaceSystem, core.NamespacePublic}

// tenantController is responsible for performing actions dependent upon a tenant phase
type tenantController struct {
	// lister that can list tenants from a shared cache
	lister corelisters.TenantLister
	// returns true when the tenant cache is ready
	listerSynced cache.InformerSynced
	// tenants that have been queued up for processing by workers
	queue workqueue.RateLimitingInterface
	// kubeclient for api calls
	kubeClient clientset.Interface
	// sync handler for injection
	syncHandler func(key string) error

	// client for network CR api calls
	networkClient arktos.Interface
	// default network spec template file path
	defaultNetworkTemplatePath string
	// templateGetter for injection
	templateGetter func(path string) (string, error)
}

// NewTenantController creates a new iinstance of tenantcontroller
func NewTenantController(
	kubeClient clientset.Interface,
	tenantInformer coreinformers.TenantInformer,
	resyncPeriod time.Duration,
	networkClient arktos.Interface,
	defaultNetworkTemplatePath string) *tenantController {

	// create the controller so we can inject the enqueue function
	tenantController := &tenantController{
		kubeClient:                 kubeClient,
		networkClient:              networkClient,
		queue:                      workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "tenant"),
		defaultNetworkTemplatePath: defaultNetworkTemplatePath,
	}

	if kubeClient != nil && kubeClient.CoreV1().RESTClient().GetRateLimiter() != nil {
		metrics.RegisterMetricAndTrackRateLimiterUsage("tenant_controller", kubeClient.CoreV1().RESTClient().GetRateLimiter())
	}

	// configure the tenant informer event handlers
	tenantInformer.Informer().AddEventHandlerWithResyncPeriod(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				tenantController.enqueue(obj)
			},
		},
		resyncPeriod,
	)
	tenantController.lister = tenantInformer.Lister()
	tenantController.listerSynced = tenantInformer.Informer().HasSynced
	tenantController.syncHandler = tenantController.syncTenant
	tenantController.templateGetter = readTemplate
	return tenantController
}

// Run starts the controller with the specified number of workers.
func (tc *tenantController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer tc.queue.ShutDown()

	klog.Infof("Starting tenant controller.")
	defer klog.Infof("Shutting down tenant controller")

	if !controller.WaitForCacheSync("tenant", stopCh, tc.listerSynced) {
		return
	}

	klog.V(5).Info("Starting workers of tenant controller")
	for i := 0; i < workers; i++ {
		go wait.Until(tc.worker, time.Second, stopCh)
	}
	<-stopCh
}

// worker processes the queue of tenant objects.
func (tc *tenantController) worker() {
	workFunc := func() bool {
		key, quit := tc.queue.Get()
		if quit {
			return true
		}
		defer tc.queue.Done(key)

		err := tc.processQueue(key.(string))
		if err == nil {
			// no error, forget this entry and return
			tc.queue.Forget(key)
			return false
		} else {
			// rather than wait for a full resync, re-add the tenant to the queue to be processed
			tc.queue.AddRateLimited(key)
			utilruntime.HandleError(err)
		}
		return false
	}

	for {
		quit := workFunc()

		if quit {
			return
		}
	}
}

// enqueue adds an object to the controller work queue
func (tc *tenantController) enqueue(obj interface{}) {
	klog.Infof("Starting tenant enque.")

	tenant, ok := obj.(*v1.Tenant)
	if !ok {
		utilruntime.HandleError(fmt.Errorf("Not a tenant object: %v", obj))
		return
	}

	tc.queue.Add(tenant.Name)
}

// processQueue looks for a tenant with the specified name and synchronizes it
func (tc *tenantController) processQueue(tenantName string) (err error) {
	klog.Infof("Starting processsing queue for tenant: %v", tenantName)

	startTime := time.Now()
	defer func() {
		klog.V(4).Infof("Finished syncing tenant %q (%v)", tenantName, time.Since(startTime))
	}()

	_, err = tc.lister.Get(tenantName)
	if errors.IsNotFound(err) {
		klog.Infof("tenant has been deleted %v", tenantName)
		return nil
	}
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("Unable to retrieve tenant %v from store: %v", tenantName, err))
		return err
	}

	return tc.syncHandler(tenantName)
}

// syncTenant creates the default resources for a new tenant
func (tc *tenantController) syncTenant(tenantName string) (err error) {
	klog.Infof("Starting tenant initialization: %v", tenantName)

	startTime := time.Now()
	defer func() {
		klog.V(4).Infof("Finished initializing tenant %q (%v)", tenantName, time.Since(startTime))
	}()

	createFailures := []error{}

	// Create Default namespaces
	for _, nsName := range tenant_default_namespaces {

		ns := v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Tenant: tenantName, Name: nsName},
		}
		if _, err := tc.kubeClient.CoreV1().NamespacesWithMultiTenancy(tenantName).Create(&ns); err != nil && !errors.IsAlreadyExists(err) {
			createFailures = append(createFailures, err)
		}
	}

	// create default network object, if applicable
	tenant, _ := tc.lister.Get(tenantName)	// no error as its caller, processQueue, has checked.
	if tenant.Status.Phase == v1.TenantTerminating {
		klog.Infof("Tenant %q is terminating; skipped the creation of default network", tenantName)
	} else if len(tc.defaultNetworkTemplatePath) == 0 {
		klog.Infof("No default network template path; skipped the creation of default network in tenant %q", tenantName)
	} else {
		klog.Infof("creating the default network in tenant %q", tenantName)
		defaultNetwork := arktosv1.Network{}
		if err = tc.getDefaultNetwork(tenantName, &defaultNetwork); err != nil {
			createFailures = append(createFailures, err)
		} else {
			if _, err = tc.networkClient.ArktosV1().NetworksWithMultiTenancy(tenantName).Create(&defaultNetwork); err != nil && !errors.IsAlreadyExists(err) {
				createFailures = append(createFailures, err)
			}
		}
	}

	if len(createFailures) != 0 {
		ret := utilerrors.Flatten(utilerrors.NewAggregate(createFailures))
		klog.Errorf("Errors happened in tenant initialization of %v: %v", tenantName, ret)
		return ret
	}

	return nil
}

func (tc *tenantController) getDefaultNetwork(tenant string, net *arktosv1.Network) error {
	// todo: validate content of template file
	tmpl, err := tc.templateGetter(tc.defaultNetworkTemplatePath)
	if err != nil {
		return err
	}
	t, err := template.New("default").Parse(tmpl)
	if err != nil {
		return err
	}

	var bytesJson bytes.Buffer
	if err = t.Execute(&bytesJson, tenant); err != nil {
		return err
	}

	return json.Unmarshal(bytesJson.Bytes(), net)
}

func readTemplate(path string) (string, error) {
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

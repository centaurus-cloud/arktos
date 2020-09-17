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

Reference:
(1) https://github.com/futurewei-cloud/arktos.git, arktos/pkg/controller/endpoints_controller
*/

package mizar

import (
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/controller"
)

const (
	EndpointsKind          string = "Endpoints"
	Endpoints_Ready        string = "True"
	EndpointsStatusMessage string = "HANDLED"
	EndpointsNoChange      int    = 1
	EndpointsUpdate        int    = 2
	EndpointsResume        int    = 3
)

type ServiceEndpoint struct {
	name      string
	namespace string
	tenant    string
	addresses []string
	ports     []Ports
}

//frontPort: service' port, backendPort: endpoint' port
//protocol: network protocol TCP by default
type Ports struct {
	frontPort   string
	backendPort string
	protocol    string
}

type MizarEndpointsController struct {
	kubeclientset       *kubernetes.Clientset
	informer            coreinformers.EndpointsInformer
	informerSynced      cache.InformerSynced
	lister              corelisters.EndpointsLister
	serviceListerSynced cache.InformerSynced
	serviceLister       corelisters.ServiceLister
	recorder            record.EventRecorder
	queue               workqueue.RateLimitingInterface
	grpcHost            string
}

func NewMizarEndpointsController(kubeclientset *kubernetes.Clientset, endpointInformer coreinformers.EndpointsInformer, serviceInformer coreinformers.ServiceInformer, grpcHost string) *MizarEndpointsController {
	informer := endpointInformer
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "mizar-endpoints-controller"})
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(
		&v1core.EventSinkImpl{Interface: kubeclientset.CoreV1().EventsWithMultiTenancy(metav1.NamespaceAll, metav1.TenantAll)})
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	c := &MizarEndpointsController{
		kubeclientset:       kubeclientset,
		informer:            informer,
		informerSynced:      informer.Informer().HasSynced,
		lister:              informer.Lister(),
		serviceListerSynced: serviceInformer.Informer().HasSynced,
		serviceLister:       serviceInformer.Lister(),
		recorder:            recorder,
		queue:               queue,
		grpcHost:            grpcHost,
	}
	klog.Infof("Sending events to api server")
	informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addEndpoints,
		UpdateFunc: c.updateEndpoints,
		DeleteFunc: c.deleteEndpoints,
	})
	return c
}

func (c *MizarEndpointsController) addEndpoints(object interface{}) {
	key, err := controller.KeyFunc(object)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", object, err))
		return
	}
	c.Enqueue(key, EventType_Create)
	klog.Infof("Create Endpoint - %v", key)
}

func (c *MizarEndpointsController) updateEndpoints(oldObject, newObject interface{}) {
	key1, err1 := controller.KeyFunc(oldObject)
	key2, err2 := controller.KeyFunc(newObject)
	if key1 == "" || key2 == "" || err1 != nil || err2 != nil {
		klog.Errorf("Unexpected string in queue; discarding - %v", key2)
		return
	}
	oldResource := oldObject.(*v1.Endpoints)
	newResource := newObject.(*v1.Endpoints)
	name := newResource.GetName()
	if name == "kube-controller-manager" || name == "kube-scheduler" {
		return
	}
	eventType, err := c.determineEventType(oldResource, newResource)
	if err != nil {
		klog.Errorf("Unexpected string in queue; discarding - %v ", key2)
		return
	}
	switch eventType {
	case EndpointsNoChange:
		{
			klog.Infof("No actual change in endpoints, discarding -%v ", key2)
			break
		}
	case EndpointsUpdate:
		{
			c.Enqueue(key2, EventType_Update)
			klog.Infof("Update Endpoints - %v", key2)
			break
		}
	case EndpointsResume:
		{
			c.Enqueue(key2, EventType_Resume)
			klog.Infof("Resume Endpoints - %v", key2)
		}
	default:
		{
			klog.Errorf("Unexpected Endpoints event; discarding - %v", key2)
			return
		}
	}
}

func (c *MizarEndpointsController) deleteEndpoints(object interface{}) {
	key, err := controller.KeyFunc(object)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", object, err))
		return
	}
	c.Enqueue(key, EventType_Delete)
}

// Run starts an asynchronous loop that detects events of cluster nodes.
func (c *MizarEndpointsController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()
	klog.Infof("Starting endpoint controller")
	klog.Infof("Waiting cache to be synced")
	if ok := cache.WaitForCacheSync(stopCh, c.informerSynced); !ok {
		klog.Infof("Timeout expired during waiting for caches to sync.")
	}
	klog.Infof("Starting workers...")
	for i := 0; i < workers; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}
	<-stopCh
	klog.Info("Shutting down endpoint controller")
}

// Enqueue puts key of the endpoints object in the work queue
// EventType: Create=0, Update=1, Delete=2, Resume=3
func (c *MizarEndpointsController) Enqueue(key string, eventType EventType) {
	c.queue.Add(KeyWithEventType{Key: key, EventType: eventType})
}

// Dequeue an item and process it
func (c *MizarEndpointsController) runWorker() {
	for {
		item, queueIsEmpty := c.queue.Get()
		if queueIsEmpty {
			return
			//break
		}
		c.process(item)
	}
}

// Parsing a item key and call gRPC request
func (c *MizarEndpointsController) process(item interface{}) {
	defer c.queue.Done(item)
	keyWithEventType, ok := item.(KeyWithEventType)
	if !ok {
		klog.Errorf("Unexpected item in queue - %v", keyWithEventType)
		c.queue.Forget(item)
		return
	}
	key := keyWithEventType.Key
	eventType := keyWithEventType.EventType
	tenant, namespace, epName, err := cache.SplitMetaTenantNamespaceKey(key)
	if err != nil {
		klog.Errorf("Unexpected string in queue; discarding: ", key)
		c.queue.Forget(item)
		return
	}
	service, err := c.serviceLister.ServicesWithMultiTenancy(namespace, tenant).Get(epName)
	if service == nil || err != nil {
		klog.Errorf("Endpoints' service not found - endpoint's name: %s", epName)
		return
	}
	ep, err := c.lister.EndpointsWithMultiTenancy(namespace, tenant).Get(epName)
	if ep == nil || err != nil {
		klog.Errorf("Failed to retrieve endpoint in local cache by namespace, tenant, name - %v, %v, %v", namespace, tenant, epName)
		c.queue.Forget(item)
		return
	}
	klog.Errorf("Endpoints controller creates gRPC request for service: %v endpoints: %v ", service.Name, ep.Name)
	result, err := c.gRPCRequest(eventType, ep, service)
	if !result {
		klog.Errorf("Failed endpoints processing -%v ", key)
		c.queue.AddRateLimited(item)
	} else {
		klog.Infof(" Processed endpoints - %v", key)
		c.queue.Forget(item)
	}
}

//Determine an event is NoChange, Update or Resume
func (c *MizarEndpointsController) determineEventType(resource1 *v1.Endpoints, resource2 *v1.Endpoints) (eventType int, err error) {
	if resource1 == nil || resource2 == nil {
		err = fmt.Errorf("It cannot determine null endpoints event type - endpoints1: %v, endpoints2:%v", resource1, resource2)
		return
	}
	epSubsets1 := resource1.Subsets
	epSubsets2 := resource2.Subsets
	subset1 := fmt.Sprintf("%v", epSubsets1)
	subset2 := fmt.Sprintf("%v", epSubsets2)
	if subset1 == subset2 {
		eventType = EndpointsNoChange
		return
	}
	//var notReadyAddressSet sets.String
	//var readyAddressSet sets.String
	notReadyAddressSet := sets.String{}
	readyAddressSet := sets.String{}
	for i := 0; i < len(epSubsets1); i++ {
		notReadyAddresses := epSubsets1[i].NotReadyAddresses
		for j := 0; j < len(notReadyAddresses); j++ {
			notReadyAddress := notReadyAddresses[j].IP
			notReadyAddressSet.Insert(notReadyAddress)
		}
	}
	for i := 0; i < len(epSubsets2); i++ {
		readyAddresses := epSubsets2[i].Addresses
		for j := 0; j < len(readyAddresses); j++ {
			readyAddress := readyAddresses[j].IP
			readyAddressSet.Insert(readyAddress)
		}
	}
	newReadyAddresses := readyAddressSet.Intersection(notReadyAddressSet)
	eventType = EndpointsUpdate
	if newReadyAddresses != nil {
		eventType = EndpointsResume
	}
	return
}

//This function returns front port, backend port, and protocol
//ServicePort: protocol, port (=service port = front port), targetPort (endpoint port = backend port)
//(e.g) ports: {protocol: TCP, port: 80,  targetPort: 9376 }
func (c *MizarEndpointsController) getPorts(namespace, tenant, epName string, epPort int32) Ports {
	var ports Ports
	service, err := c.serviceLister.ServicesWithMultiTenancy(namespace, tenant).Get(epName)
	if err != nil {
		klog.Errorf("Service not found - %v", epName)
		return ports
	}
	serviceports := service.Spec.Ports
	if serviceports == nil {
		klog.Errorf("Service ports are not found - %v", epName)
		return ports
	}
	for i := 0; i < len(serviceports); i++ {
		serviceport := serviceports[i]
		targetPort := serviceport.TargetPort.IntVal
		if targetPort == epPort {
			ports.frontPort = fmt.Sprintf("%v", serviceport.Port)
			ports.backendPort = fmt.Sprintf("%v", serviceport.TargetPort)
			ports.protocol = fmt.Sprintf("%v", serviceport.Protocol)
			return ports
		}
	}
	return ports
}

//gRPC request message, Integration is needed
func (c *MizarEndpointsController) gRPCRequest(event EventType, ep *v1.Endpoints, service *v1.Service) (response bool, err error) {
	switch event {
	case EventType_Create:
		response := GrpcCreateServiceEndpointFront(c.grpcHost, ep, service)
		if response.Code != CodeType_OK {
			klog.Errorf("Endpoint creation failed on Mizar side - %v, %v", response.Code, err)
			return false, err
		}
	case EventType_Update:
		response := GrpcUpdateServiceEndpointFront(c.grpcHost, ep, service)
		if response.Code != CodeType_OK {
			klog.Errorf("Endpoint update failed on Mizar side - %v", err)
			return false, err
		}
	case EventType_Resume:
		response := GrpcResumeServiceEndpointFront(c.grpcHost, ep, service)
		if response.Code != CodeType_OK {
			klog.Errorf("Endpoint resume failed on Mizar side - %v", err)
			return false, err
		}
	default:
		klog.Errorf("gRPC event is not correct", event)
		err = fmt.Errorf("gRPC event is not correct - %v", event)
		return false, err
	}
	klog.Infof("gRPC request is sent")
	return true, nil
}
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

package app

import (
	"net/http"
	"time"

	informers "k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	controllers "k8s.io/kubernetes/pkg/controller/mizar"
)

const (
	mizarStarterControllerWorkerCount   = 2
	mizarPodControllerWorkerCount       = 4
	mizarNodeControllerWorkerCount      = 4
	mizarEndpointsControllerWorkerCount = 4
)

func startMizarStarterController(ctx ControllerContext) (http.Handler, bool, error) {
	controllerName := "mizar-starter-controller"
	klog.V(2).Infof("Starting %v", controllerName)

	go controllers.NewMizarStarterController(
		ctx.InformerFactory.Core().V1().ConfigMaps(),
		ctx.ClientBuilder.ClientOrDie(controllerName),
		ctx,
		startHandler,
	).Run(mizarStarterControllerWorkerCount, ctx.Stop)
	return nil, true, nil
}

func startHandler(controllerContext interface{}, grpcHost string) {
	ctx := controllerContext.(ControllerContext)
	startMizarPodController(&ctx, grpcHost)
	startMizarNodeController(&ctx, grpcHost)
}

func startMizarPodController(ctx *ControllerContext, grpcHost string) (http.Handler, bool, error) {
	controllerName := "mizar-pod-controller"
	klog.V(2).Infof("Starting %v", controllerName)

	go controllers.NewMizarPodController(
		ctx.InformerFactory.Core().V1().Pods(),
		ctx.ClientBuilder.ClientOrDie(controllerName),
		grpcHost,
	).Run(mizarPodControllerWorkerCount, ctx.Stop)
	return nil, true, nil
}

func startMizarNodeController(ctx *ControllerContext, grpcHost string) (err error) {
	controllerName := "mizar-node-controller"
	klog.V(2).Infof("Starting %v", controllerName)

	nodeKubeconfigs := ctx.ClientBuilder.ConfigOrDie(controllerName)
	nodeKubeClient := clientset.NewForConfigOrDie(nodeKubeconfigs)
	informerFactory := informers.NewSharedInformerFactory(nodeKubeClient, 10*time.Minute)
	nodeInformer := informerFactory.Core().V1().Nodes()
	nodeController, err := controllers.NewMizarNodeController(nodeKubeClient, nodeInformer, grpcHost)
	if err != nil {
		klog.Fatalf("Error in building mizar node controller: %v", err.Error())
	}
	go nodeController.Run(mizarNodeControllerWorkerCount, ctx.Stop)
	return err
}

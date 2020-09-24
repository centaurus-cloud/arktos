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

package mizar

import (
	"context"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/controller/testutil"
)

const (
	testGrpcHost = "10.0.1.17"

	mizarNodeControllerWorkerCount = 2
)

func TestCreate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mizarNodeController, _, nodeInformer := getNewMizarNodeController()
	mizarNodeController.listerSynced = alwaysReady
	go mizarNodeController.Run(mizarNodeControllerWorkerCount, ctx.Done())

	list := nodeInformer.Informer().GetStore().List()
	go nodeInformer.Informer().Run(ctx.Done())
	print(list)
	time.Sleep(time.Second * 2) // TODO don't use sleep
	// TODO use fake grpc-adaptor
}

func TestUpdate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mizarNodeController, kubeClient, nodeInformer := getNewMizarNodeController()
	mizarNodeController.listerSynced = alwaysReady
	go mizarNodeController.Run(mizarNodeControllerWorkerCount, ctx.Done())

	syncNodeStore(nodeInformer, kubeClient)
	go nodeInformer.Informer().Run(ctx.Done())
	list := nodeInformer.Informer().GetStore().List()
	print(list)
	time.Sleep(time.Second * 2) // TODO don't use sleep
}

func syncNodeStore(nodeInformer coreinformers.NodeInformer, kubeClient *testutil.FakeNodeHandler) error {
	list := nodeInformer.Informer().GetStore().List()
	print(list)
	nodes, err := kubeClient.List(metav1.ListOptions{})
	nodes.Items[0].ResourceVersion = "old version"
	if err != nil {
		return err
	}
	newElems := make([]interface{}, 0, len(nodes.Items))
	for i := range nodes.Items {
		newElems = append(newElems, &nodes.Items[i])
	}
	return nodeInformer.Informer().GetStore().Replace(newElems, "newRV")
}

func getNewMizarNodeController() (*MizarNodeController, *testutil.FakeNodeHandler, coreinformers.NodeInformer) {
	kubeClient :=
		&testutil.FakeNodeHandler{
			Existing: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
						Labels: map[string]string{
							v1.LabelZoneRegion:        "region1",
							v1.LabelZoneFailureDomain: "zone1",
						},
						ResourceVersion: "test version",
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionTrue,
								LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
								LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							},
						},
					},
				},
			},
			DeletedNodes: []*v1.Node{},
			Clientset:    fake.NewSimpleClientset(),
		}
	factory := informers.NewSharedInformerFactory(kubeClient, controller.NoResyncPeriodFunc())
	nodeInformer := factory.Core().V1().Nodes()

	kubeClient.CreateHook = func(c *testutil.FakeNodeHandler, n *v1.Node) bool { return true }
	return NewMizarNodeController(nodeInformer, kubeClient, testGrpcHost), kubeClient, nodeInformer
}

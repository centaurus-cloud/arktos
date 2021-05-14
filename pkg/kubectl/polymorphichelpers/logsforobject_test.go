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

package polymorphichelpers

import (
	"reflect"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/diff"
	fakeexternal "k8s.io/client-go/kubernetes/fake"
	testclient "k8s.io/client-go/testing"
)

var (
	podsResource = schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	podsKind     = schema.GroupVersionKind{Version: "v1", Kind: "Pod"}
)

func TestLogsForObject(t *testing.T) {
	testLogsForObject(t, metav1.TenantSystem)
}

func TestLogsForObjectWithMultiTenancy(t *testing.T) {
	testLogsForObject(t, metav1.TenantSystem)
}

func testLogsForObject(t *testing.T, tenant string) {
	tests := []struct {
		name          string
		obj           runtime.Object
		opts          *corev1.PodLogOptions
		allContainers bool
		pods          []runtime.Object
		actions       []testclient.Action
	}{
		{
			name: "pod logs",
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "test", Tenant: tenant},
			},
			pods: []runtime.Object{testPod(tenant)},
			actions: []testclient.Action{
				getLogsAction(tenant, "test", nil),
			},
		},
		{
			name: "pod logs: all containers",
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "test", Tenant: tenant},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{Name: "initc1"},
						{Name: "initc2"},
					},
					Containers: []corev1.Container{
						{Name: "c1"},
						{Name: "c2"},
					},
				},
			},
			opts:          &corev1.PodLogOptions{},
			allContainers: true,
			pods:          []runtime.Object{testPod(tenant)},
			actions: []testclient.Action{
				getLogsAction(tenant, "test", &corev1.PodLogOptions{Container: "initc1"}),
				getLogsAction(tenant, "test", &corev1.PodLogOptions{Container: "initc2"}),
				getLogsAction(tenant, "test", &corev1.PodLogOptions{Container: "c1"}),
				getLogsAction(tenant, "test", &corev1.PodLogOptions{Container: "c2"}),
			},
		},
		{
			name: "pods list logs",
			obj: &corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "test", Tenant: tenant},
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{
								{Name: "initc1"},
								{Name: "initc2"},
							},
							Containers: []corev1.Container{
								{Name: "c1"},
								{Name: "c2"},
							},
						},
					},
				},
			},
			pods: []runtime.Object{testPod(tenant)},
			actions: []testclient.Action{
				getLogsAction(tenant, "test", nil),
			},
		},
		{
			name: "pods list logs: all containers",
			obj: &corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "test", Tenant: tenant},
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{
								{Name: "initc1"},
								{Name: "initc2"},
							},
							Containers: []corev1.Container{
								{Name: "c1"},
								{Name: "c2"},
							},
						},
					},
				},
			},
			opts:          &corev1.PodLogOptions{},
			allContainers: true,
			pods:          []runtime.Object{testPod(tenant)},
			actions: []testclient.Action{
				getLogsAction(tenant, "test", &corev1.PodLogOptions{Container: "initc1"}),
				getLogsAction(tenant, "test", &corev1.PodLogOptions{Container: "initc2"}),
				getLogsAction(tenant, "test", &corev1.PodLogOptions{Container: "c1"}),
				getLogsAction(tenant, "test", &corev1.PodLogOptions{Container: "c2"}),
			},
		},
		{
			name: "replication controller logs",
			obj: &corev1.ReplicationController{
				ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "test", Tenant: tenant},
				Spec: corev1.ReplicationControllerSpec{
					Selector: map[string]string{"foo": "bar"},
				},
			},
			pods: []runtime.Object{testPod(tenant)},
			actions: []testclient.Action{
				testclient.NewListActionWithMultiTenancy(podsResource, podsKind, "test", metav1.ListOptions{LabelSelector: "foo=bar"}, metav1.TenantSystem),
				getLogsAction(tenant, "test", nil),
			},
		},
		{
			name: "replica set logs",
			obj: &extensionsv1beta1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "test", Tenant: tenant},
				Spec: extensionsv1beta1.ReplicaSetSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
				},
			},
			pods: []runtime.Object{testPod(tenant)},
			actions: []testclient.Action{
				testclient.NewListActionWithMultiTenancy(podsResource, podsKind, "test", metav1.ListOptions{LabelSelector: "foo=bar"}, metav1.TenantSystem),
				getLogsAction(tenant, "test", nil),
			},
		},
		{
			name: "deployment logs",
			obj: &extensionsv1beta1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "test", Tenant: tenant},
				Spec: extensionsv1beta1.DeploymentSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
				},
			},
			pods: []runtime.Object{testPod(tenant)},
			actions: []testclient.Action{
				testclient.NewListActionWithMultiTenancy(podsResource, podsKind, "test", metav1.ListOptions{LabelSelector: "foo=bar"}, metav1.TenantSystem),
				getLogsAction(tenant, "test", nil),
			},
		},
		{
			name: "job logs",
			obj: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "test", Tenant: tenant},
				Spec: batchv1.JobSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
				},
			},
			pods: []runtime.Object{testPod(tenant)},
			actions: []testclient.Action{
				testclient.NewListActionWithMultiTenancy(podsResource, podsKind, "test", metav1.ListOptions{LabelSelector: "foo=bar"}, metav1.TenantSystem),
				getLogsAction(tenant, "test", nil),
			},
		},
		{
			name: "stateful set logs",
			obj: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "test", Tenant: tenant},
				Spec: appsv1.StatefulSetSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
				},
			},
			pods: []runtime.Object{testPod(tenant)},
			actions: []testclient.Action{
				testclient.NewListActionWithMultiTenancy(podsResource, podsKind, "test", metav1.ListOptions{LabelSelector: "foo=bar"}, metav1.TenantSystem),
				getLogsAction(tenant, "test", nil),
			},
		},
	}

	for _, test := range tests {
		fakeClientset := fakeexternal.NewSimpleClientset(test.pods...)
		_, err := logsForObjectWithClient(fakeClientset.CoreV1(), test.obj, test.opts, 20*time.Second, test.allContainers)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", test.name, err)
			continue
		}

		for i := range test.actions {
			if len(fakeClientset.Actions()) < i {
				t.Errorf("%s: action %d does not exists in actual actions: %#v",
					test.name, i, fakeClientset.Actions())
				continue
			}
			got := fakeClientset.Actions()[i]
			want := test.actions[i]
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s: unexpected action: %s", test.name, diff.ObjectDiff(got, want))
			}
		}
	}
}

func testPod(tenant string) runtime.Object {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "test",
			Tenant:    tenant,
			Labels:    map[string]string{"foo": "bar"},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyAlways,
			DNSPolicy:     corev1.DNSClusterFirst,
			Containers: []corev1.Container{
				{Name: "c1"},
				{Name: "c2"},
			},
		},
	}
}

func getLogsAction(tenant, namespace string, opts *corev1.PodLogOptions) testclient.Action {
	action := testclient.GenericActionImpl{}
	action.Verb = "get"
	action.Namespace = namespace
	action.Tenant = tenant
	action.Resource = podsResource
	action.Subresource = "log"
	action.Value = opts
	return action
}

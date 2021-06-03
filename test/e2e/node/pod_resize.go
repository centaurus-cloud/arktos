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

package node

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/component-base/featuregate"
	kubecm "k8s.io/kubernetes/pkg/kubelet/cm"

	"k8s.io/kubernetes/test/e2e/framework"
	imageutils "k8s.io/kubernetes/test/utils/image"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

const (
	InPlacePodVerticalScalingFeature featuregate.Feature = "InPlacePodVerticalScaling"

	CgroupCPUPeriod string = "/sys/fs/cgroup/cpu/cpu.cfs_period_us"
	CgroupCPUShares string = "/sys/fs/cgroup/cpu/cpu.shares"
	CgroupCPUQuota  string = "/sys/fs/cgroup/cpu/cpu.cfs_quota_us"
	CgroupMemLimit  string = "/sys/fs/cgroup/memory/memory.limit_in_bytes"

	PollInterval time.Duration = 2 * time.Second
	PollTimeout  time.Duration = 2 * time.Minute
)

type ContainerResources struct {
	CPUReq, CPULim, MemReq, MemLim, EphStorReq, EphStorLim string
}

type ContainerAllocations struct {
	CPUAlloc, MemAlloc, ephStorAlloc string
}

type TestContainerInfo struct {
	Name        string
	Resources   *ContainerResources
	Allocations *ContainerAllocations
	CPUPolicy   *v1.ContainerResizePolicy
	MemPolicy   *v1.ContainerResizePolicy
}

func getTestResourceInfo(tcInfo TestContainerInfo) (v1.ResourceRequirements, v1.ResourceList, []v1.ResizePolicy) {
	var res v1.ResourceRequirements
	var alloc v1.ResourceList
	var resizePol []v1.ResizePolicy

	if tcInfo.Resources != nil {
		var lim, req v1.ResourceList
		if tcInfo.Resources.CPULim != "" || tcInfo.Resources.MemLim != "" || tcInfo.Resources.EphStorLim != "" {
			lim = make(v1.ResourceList)
		}
		if tcInfo.Resources.CPUReq != "" || tcInfo.Resources.MemReq != "" || tcInfo.Resources.EphStorReq != "" {
			req = make(v1.ResourceList)
		}
		if tcInfo.Resources.CPULim != "" {
			lim[v1.ResourceCPU] = resource.MustParse(tcInfo.Resources.CPULim)
		}
		if tcInfo.Resources.MemLim != "" {
			lim[v1.ResourceMemory] = resource.MustParse(tcInfo.Resources.MemLim)
		}
		if tcInfo.Resources.EphStorLim != "" {
			lim[v1.ResourceEphemeralStorage] = resource.MustParse(tcInfo.Resources.EphStorLim)
		}
		if tcInfo.Resources.CPUReq != "" {
			req[v1.ResourceCPU] = resource.MustParse(tcInfo.Resources.CPUReq)
		}
		if tcInfo.Resources.MemReq != "" {
			req[v1.ResourceMemory] = resource.MustParse(tcInfo.Resources.MemReq)
		}
		if tcInfo.Resources.EphStorReq != "" {
			req[v1.ResourceEphemeralStorage] = resource.MustParse(tcInfo.Resources.EphStorReq)
		}
		res = v1.ResourceRequirements{Limits: lim, Requests: req}
	}
	if tcInfo.Allocations != nil {
		alloc = make(v1.ResourceList)
		if tcInfo.Allocations.CPUAlloc != "" {
			alloc[v1.ResourceCPU] = resource.MustParse(tcInfo.Allocations.CPUAlloc)
		}
		if tcInfo.Allocations.MemAlloc != "" {
			alloc[v1.ResourceMemory] = resource.MustParse(tcInfo.Allocations.MemAlloc)
		}
		if tcInfo.Allocations.ephStorAlloc != "" {
			alloc[v1.ResourceEphemeralStorage] = resource.MustParse(tcInfo.Allocations.ephStorAlloc)
		}

	}
	if tcInfo.CPUPolicy != nil {
		cpuPol := v1.ResizePolicy{ResourceName: v1.ResourceCPU, Policy: *tcInfo.CPUPolicy}
		resizePol = append(resizePol, cpuPol)
	}
	if tcInfo.MemPolicy != nil {
		memPol := v1.ResizePolicy{ResourceName: v1.ResourceMemory, Policy: *tcInfo.MemPolicy}
		resizePol = append(resizePol, memPol)
	}
	return res, alloc, resizePol
}

func makeTestContainer(tcInfo TestContainerInfo) v1.Container {
	cmd := "trap exit TERM; while true; do sleep 1; done"
	res, alloc, resizePol := getTestResourceInfo(tcInfo)
	tc := v1.Container{
		Name:               tcInfo.Name,
		Image:              imageutils.GetE2EImage(imageutils.BusyBox),
		Command:            []string{"/bin/sh"},
		Args:               []string{"-c", cmd},
		Resources:          res,
		ResourcesAllocated: alloc,
		ResizePolicy:       resizePol,
	}
	return tc
}

func makeTestPod(ns, name, timeStamp string, tcInfo []TestContainerInfo) *v1.Pod {
	var testContainers []v1.Container
	for _, ci := range tcInfo {
		tc := makeTestContainer(ci)
		testContainers = append(testContainers, tc)
	}
	pod := &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels: map[string]string{
				"name": "fooPod",
				"time": timeStamp,
			},
		},
		Spec: v1.PodSpec{
			Containers:    testContainers,
			RestartPolicy: v1.RestartPolicyOnFailure,
		},
	}
	return pod
}

func makeTestVM(tcInfo TestContainerInfo) v1.VirtualMachine {
	res, alloc, resizePol := getTestResourceInfo(tcInfo)
	tvm := v1.VirtualMachine{
		Name:               tcInfo.Name,
		KeyPairName:        "foobar",
		Image:              "download.cirros-cloud.net/0.3.5/cirros-0.3.5-x86_64-disk.img",
		Resources:          res,
		ResourcesAllocated: alloc,
		ResizePolicy:       resizePol,
	}
	return tvm
}

func makeTestVMPod(ns, name, timeStamp string, tcInfo []TestContainerInfo) *v1.Pod {
	tvm := makeTestVM(tcInfo[0])
	pod := &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels: map[string]string{
				"name": "fooPod",
				"time": timeStamp,
			},
		},
		Spec: v1.PodSpec{
			VirtualMachine: &tvm,
			RestartPolicy:  v1.RestartPolicyOnFailure,
		},
	}
	return pod
}

func verifyPodResizePolicy(pod *v1.Pod, tcInfo []TestContainerInfo) {
	wMap := make(map[string]*v1.CommonInfo)
	for i, w := range pod.Spec.Workloads() {
		wMap[w.Name] = &pod.Spec.WorkloadInfo[i]
	}
	for _, ci := range tcInfo {
		w, found := wMap[ci.Name]
		gomega.Expect(found == true)
		tc := makeTestContainer(ci)
		framework.ExpectEqual(w.ResizePolicy, tc.ResizePolicy)
	}
}

func verifyPodResources(pod *v1.Pod, tcInfo []TestContainerInfo) {
	wMap := make(map[string]*v1.CommonInfo)
	for i, w := range pod.Spec.Workloads() {
		wMap[w.Name] = &pod.Spec.WorkloadInfo[i]
	}
	for _, ci := range tcInfo {
		w, found := wMap[ci.Name]
		gomega.Expect(found == true)
		tc := makeTestContainer(ci)
		framework.ExpectEqual(w.Resources, tc.Resources)
	}
}

func verifyPodAllocations(pod *v1.Pod, tcInfo []TestContainerInfo) {
	wMap := make(map[string]*v1.CommonInfo)
	for i, w := range pod.Spec.Workloads() {
		wMap[w.Name] = &pod.Spec.WorkloadInfo[i]
	}
	for _, ci := range tcInfo {
		w, found := wMap[ci.Name]
		gomega.Expect(found == true)
		if ci.Allocations == nil {
			alloc := &ContainerAllocations{CPUAlloc: ci.Resources.CPUReq, MemAlloc: ci.Resources.MemReq}
			ci.Allocations = alloc
			defer func() {
				ci.Allocations = nil
			}()
		}
		tc := makeTestContainer(ci)
		framework.ExpectEqual(w.ResourcesAllocated, tc.ResourcesAllocated)
	}
}

func verifyPodStatusResources(pod *v1.Pod, tcInfo []TestContainerInfo) {
	if pod.Spec.VirtualMachine == nil {
		csMap := make(map[string]*v1.ContainerStatus)
		for i, c := range pod.Status.ContainerStatuses {
			csMap[c.Name] = &pod.Status.ContainerStatuses[i]
		}
		for _, ci := range tcInfo {
			cs, found := csMap[ci.Name]
			gomega.Expect(found == true)
			tc := makeTestContainer(ci)
			framework.ExpectEqual(cs.Resources, tc.Resources)
		}
	} else {
		gomega.Expect(pod.Status.VirtualMachineStatus != nil)
		tc := makeTestContainer(tcInfo[0])
		framework.ExpectEqual(pod.Status.VirtualMachineStatus.Resources, tc.Resources)
	}
}

func verifyPodContainersCgroupConfig(pod *v1.Pod, tcInfo []TestContainerInfo) {
	if pod.Spec.VirtualMachine != nil {
		//TODO: Fix kubectl exec so that it works for VMs, and then remove this return
		return
	}
	verifyCgroupValue := func(cName, cgPath, expectedCgValue string) {
		cmd := []string{"head", "-n", "1", cgPath}
		cgValue, err := framework.LookForStringInPodExecToContainer(pod.Namespace, pod.Name, cName, cmd, expectedCgValue, PollTimeout)
		framework.ExpectNoError(err, "failed to find expected cgroup value in container")
		cgValue = strings.Trim(cgValue, "\n")
		gomega.Expect(cgValue == expectedCgValue)
	}
	for _, ci := range tcInfo {
		if ci.Resources == nil {
			continue
		}
		tc := makeTestContainer(ci)
		if tc.Resources.Limits != nil || tc.Resources.Requests != nil {
			var cpuShares int64
			memLimitInBytes := tc.Resources.Limits.Memory().Value()
			cpuRequest := tc.Resources.Requests.Cpu()
			cpuLimit := tc.Resources.Limits.Cpu()
			if cpuRequest.IsZero() && !cpuLimit.IsZero() {
				cpuShares = int64(kubecm.MilliCPUToShares(cpuLimit.MilliValue()))
			} else {
				cpuShares = int64(kubecm.MilliCPUToShares(cpuRequest.MilliValue()))
			}
			cpuQuota := kubecm.MilliCPUToQuota(cpuLimit.MilliValue(), kubecm.QuotaPeriod)
			if cpuLimit.IsZero() {
				cpuQuota = -1
			}
			verifyCgroupValue(ci.Name, CgroupCPUShares, strconv.FormatInt(cpuShares, 10))
			verifyCgroupValue(ci.Name, CgroupCPUQuota, strconv.FormatInt(cpuQuota, 10))
			if memLimitInBytes > 0 {
				verifyCgroupValue(ci.Name, CgroupMemLimit, strconv.FormatInt(memLimitInBytes, 10))
			}
		}
	}
}

func doPodResizeTest(vmPod bool) {
	f := framework.NewDefaultFramework("pod-resize")
	var podClient *framework.PodClient
	var ns string

	ginkgo.BeforeEach(func() {
		podClient = f.PodClient()
		ns = f.Namespace.Name
	})

	type testCase struct {
		name        string
		containers  []TestContainerInfo
		patchString string
		expected    []TestContainerInfo
	}

	noRestart := v1.NoRestart
	tests := []testCase{
		{
			name: "Guaranteed QoS pod, one container - increase CPU & memory",
			containers: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "100m", CPULim: "100m", MemReq: "200Mi", MemLim: "200Mi"},
					CPUPolicy: &noRestart,
					MemPolicy: &noRestart,
				},
			},
			patchString: `{"spec":{"containers":[
					{"name":"c1", "resources":{"requests":{"cpu":"200m","memory":"400Mi"},"limits":{"cpu":"200m","memory":"400Mi"}}}
				]}}`,
			expected: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "200m", CPULim: "200m", MemReq: "400Mi", MemLim: "400Mi"},
					CPUPolicy: &noRestart,
					MemPolicy: &noRestart,
				},
			},
		},
		{
			name: "Guaranteed QoS pod, one container - decrease CPU & memory",
			containers: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "300m", CPULim: "300m", MemReq: "500Mi", MemLim: "500Mi"},
					CPUPolicy: &noRestart,
					MemPolicy: &noRestart,
				},
			},
			patchString: `{"spec":{"containers":[
				{"name":"c1", "resources":{"requests":{"cpu":"100m","memory":"250Mi"},"limits":{"cpu":"100m","memory":"250Mi"}}}
			]}}`,
			expected: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "100m", CPULim: "100m", MemReq: "250Mi", MemLim: "250Mi"},
					CPUPolicy: &noRestart,
					MemPolicy: &noRestart,
				},
			},
		},
		{
			name: "Guaranteed QoS pod, one container - increase CPU & decrease memory",
			containers: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "100m", CPULim: "100m", MemReq: "200Mi", MemLim: "200Mi"},
				},
			},
			patchString: `{"spec":{"containers":[
                                {"name":"c1", "resources":{"requests":{"cpu":"200m","memory":"100Mi"},"limits":{"cpu":"200m","memory":"100Mi"}}}
                        ]}}`,
			expected: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "200m", CPULim: "200m", MemReq: "100Mi", MemLim: "100Mi"},
				},
			},
		},
		{
			name: "Guaranteed QoS pod, one container - decrease CPU & increase memory",
			containers: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "100m", CPULim: "100m", MemReq: "200Mi", MemLim: "200Mi"},
				},
			},
			patchString: `{"spec":{"containers":[
                                {"name":"c1", "resources":{"requests":{"cpu":"50m","memory":"300Mi"},"limits":{"cpu":"50m","memory":"300Mi"}}}
                        ]}}`,
			expected: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "50m", CPULim: "50m", MemReq: "300Mi", MemLim: "300Mi"},
				},
			},
		},
		{
			name: "Guaranteed QoS pod, three containers (c1, c2, c3) - increase: CPU (c1,c3), memory (c2) ; decrease: CPU (c2), memory (c1,c3)",
			containers: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "100m", CPULim: "100m", MemReq: "100Mi", MemLim: "100Mi"},
					CPUPolicy: &noRestart,
					MemPolicy: &noRestart,
				},
				{
					Name:      "c2",
					Resources: &ContainerResources{CPUReq: "200m", CPULim: "200m", MemReq: "200Mi", MemLim: "200Mi"},
					CPUPolicy: &noRestart,
					MemPolicy: &noRestart,
				},
				{
					Name:      "c3",
					Resources: &ContainerResources{CPUReq: "300m", CPULim: "300m", MemReq: "300Mi", MemLim: "300Mi"},
					CPUPolicy: &noRestart,
					MemPolicy: &noRestart,
				},
			},
			patchString: `{"spec":{"containers":[
					{"name":"c1", "resources":{"requests":{"cpu":"140m","memory":"50Mi"},"limits":{"cpu":"140m","memory":"50Mi"}}},
					{"name":"c2", "resources":{"requests":{"cpu":"150m","memory":"240Mi"},"limits":{"cpu":"150m","memory":"240Mi"}}},
					{"name":"c3", "resources":{"requests":{"cpu":"340m","memory":"250Mi"},"limits":{"cpu":"340m","memory":"250Mi"}}}
				]}}`,
			expected: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "140m", CPULim: "140m", MemReq: "50Mi", MemLim: "50Mi"},
					CPUPolicy: &noRestart,
					MemPolicy: &noRestart,
				},
				{
					Name:      "c2",
					Resources: &ContainerResources{CPUReq: "150m", CPULim: "150m", MemReq: "240Mi", MemLim: "240Mi"},
					CPUPolicy: &noRestart,
					MemPolicy: &noRestart,
				},
				{
					Name:      "c3",
					Resources: &ContainerResources{CPUReq: "340m", CPULim: "340m", MemReq: "250Mi", MemLim: "250Mi"},
					CPUPolicy: &noRestart,
					MemPolicy: &noRestart,
				},
			},
		},
		{
			name: "Burstable QoS pod, one container with cpu & memory requests + limits - decrease memory requests only",
			containers: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "200m", CPULim: "400m", MemReq: "250Mi", MemLim: "500Mi"},
				},
			},
			patchString: `{"spec":{"containers":[
                                {"name":"c1", "resources":{"requests":{"memory":"200Mi"}}}
                        ]}}`,
			expected: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "200m", CPULim: "400m", MemReq: "200Mi", MemLim: "500Mi"},
				},
			},
		},
		{
			name: "Burstable QoS pod, one container with cpu & memory requests + limits - decrease memory limits only",
			containers: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "200m", CPULim: "400m", MemReq: "250Mi", MemLim: "500Mi"},
				},
			},
			patchString: `{"spec":{"containers":[
                                {"name":"c1", "resources":{"limits":{"memory":"400Mi"}}}
                        ]}}`,
			expected: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "200m", CPULim: "400m", MemReq: "250Mi", MemLim: "400Mi"},
				},
			},
		},
		{
			name: "Burstable QoS pod, one container with cpu & memory requests + limits - increase memory requests only",
			containers: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "200m", CPULim: "400m", MemReq: "250Mi", MemLim: "500Mi"},
				},
			},
			patchString: `{"spec":{"containers":[
                                {"name":"c1", "resources":{"requests":{"memory":"300Mi"}}}
                        ]}}`,
			expected: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "200m", CPULim: "400m", MemReq: "300Mi", MemLim: "500Mi"},
				},
			},
		},
		{
			name: "Burstable QoS pod, one container with cpu & memory requests + limits - increase memory limits only",
			containers: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "200m", CPULim: "400m", MemReq: "250Mi", MemLim: "500Mi"},
				},
			},
			patchString: `{"spec":{"containers":[
                                {"name":"c1", "resources":{"limits":{"memory":"600Mi"}}}
                        ]}}`,
			expected: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "200m", CPULim: "400m", MemReq: "250Mi", MemLim: "600Mi"},
				},
			},
		},
		{
			name: "Burstable QoS pod, one container with cpu & memory requests + limits - decrease CPU requests only",
			containers: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "200m", CPULim: "400m", MemReq: "250Mi", MemLim: "500Mi"},
				},
			},
			patchString: `{"spec":{"containers":[
                                {"name":"c1", "resources":{"requests":{"cpu":"100m"}}}
                        ]}}`,
			expected: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "100m", CPULim: "400m", MemReq: "250Mi", MemLim: "500Mi"},
				},
			},
		},
		{
			name: "Burstable QoS pod, one container with cpu & memory requests + limits - decrease CPU limits only",
			containers: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "200m", CPULim: "400m", MemReq: "250Mi", MemLim: "500Mi"},
				},
			},
			patchString: `{"spec":{"containers":[
                                {"name":"c1", "resources":{"limits":{"cpu":"300m"}}}
                        ]}}`,
			expected: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "200m", CPULim: "300m", MemReq: "250Mi", MemLim: "500Mi"},
				},
			},
		},
		{
			name: "Burstable QoS pod, one container with cpu & memory requests + limits - increase CPU requests only",
			containers: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "100m", CPULim: "200m", MemReq: "250Mi", MemLim: "500Mi"},
				},
			},
			patchString: `{"spec":{"containers":[
                                {"name":"c1", "resources":{"requests":{"cpu":"150m"}}}
                        ]}}`,
			expected: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "150m", CPULim: "200m", MemReq: "250Mi", MemLim: "500Mi"},
				},
			},
		},
		{
			name: "Burstable QoS pod, one container with cpu & memory requests + limits - increase CPU limits only",
			containers: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "200m", CPULim: "400m", MemReq: "250Mi", MemLim: "500Mi"},
				},
			},
			patchString: `{"spec":{"containers":[
                                {"name":"c1", "resources":{"limits":{"cpu":"500m"}}}
                        ]}}`,
			expected: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "200m", CPULim: "500m", MemReq: "250Mi", MemLim: "500Mi"},
				},
			},
		},
		{
			name: "Burstable QoS pod, one container with cpu & memory requests + limits - decrease CPU requests and limits",
			containers: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "200m", CPULim: "400m", MemReq: "250Mi", MemLim: "500Mi"},
				},
			},
			patchString: `{"spec":{"containers":[
                                {"name":"c1", "resources":{"requests":{"cpu":"100m"},"limits":{"cpu":"200m"}}}
                        ]}}`,
			expected: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "100m", CPULim: "200m", MemReq: "250Mi", MemLim: "500Mi"},
				},
			},
		},
		{
			name: "Burstable QoS pod, one container with cpu & memory requests + limits - increase CPU requests and limits",
			containers: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "100m", CPULim: "200m", MemReq: "250Mi", MemLim: "500Mi"},
				},
			},
			patchString: `{"spec":{"containers":[
                                {"name":"c1", "resources":{"requests":{"cpu":"200m"},"limits":{"cpu":"400m"}}}
                        ]}}`,
			expected: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "200m", CPULim: "400m", MemReq: "250Mi", MemLim: "500Mi"},
				},
			},
		},
		{
			name: "Burstable QoS pod, one container with cpu & memory requests + limits - decrease CPU requests and increase CPU limits",
			containers: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "200m", CPULim: "400m", MemReq: "250Mi", MemLim: "500Mi"},
				},
			},
			patchString: `{"spec":{"containers":[
                                {"name":"c1", "resources":{"requests":{"cpu":"100m"},"limits":{"cpu":"500m"}}}
                        ]}}`,
			expected: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "100m", CPULim: "500m", MemReq: "250Mi", MemLim: "500Mi"},
				},
			},
		},
		{
			name: "Burstable QoS pod, one container with cpu & memory requests + limits - increase CPU requests and decrease CPU limits",
			containers: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "100m", CPULim: "400m", MemReq: "250Mi", MemLim: "500Mi"},
				},
			},
			patchString: `{"spec":{"containers":[
                                {"name":"c1", "resources":{"requests":{"cpu":"200m"},"limits":{"cpu":"300m"}}}
                        ]}}`,
			expected: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "200m", CPULim: "300m", MemReq: "250Mi", MemLim: "500Mi"},
				},
			},
		},
		{
			name: "Burstable QoS pod, one container with cpu & memory requests + limits - decrease memory requests and limits",
			containers: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "100m", CPULim: "200m", MemReq: "200Mi", MemLim: "400Mi"},
				},
			},
			patchString: `{"spec":{"containers":[
                                {"name":"c1", "resources":{"requests":{"memory":"100Mi"},"limits":{"memory":"300Mi"}}}
                        ]}}`,
			expected: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "100m", CPULim: "200m", MemReq: "100Mi", MemLim: "300Mi"},
				},
			},
		},
		{
			name: "Burstable QoS pod, one container with cpu & memory requests + limits - increase memory requests and limits",
			containers: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "100m", CPULim: "200m", MemReq: "200Mi", MemLim: "400Mi"},
				},
			},
			patchString: `{"spec":{"containers":[
                                {"name":"c1", "resources":{"requests":{"memory":"300Mi"},"limits":{"memory":"500Mi"}}}
                        ]}}`,
			expected: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "100m", CPULim: "200m", MemReq: "300Mi", MemLim: "500Mi"},
				},
			},
		},
		{
			name: "Burstable QoS pod, one container with cpu & memory requests + limits - decrease memory requests and increase memory limits",
			containers: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "100m", CPULim: "200m", MemReq: "200Mi", MemLim: "400Mi"},
				},
			},
			patchString: `{"spec":{"containers":[
                                {"name":"c1", "resources":{"requests":{"memory":"100Mi"},"limits":{"memory":"500Mi"}}}
                        ]}}`,
			expected: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "100m", CPULim: "200m", MemReq: "100Mi", MemLim: "500Mi"},
				},
			},
		},
		{
			name: "Burstable QoS pod, one container with cpu & memory requests + limits - increase memory requests and decrease memory limits",
			containers: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "100m", CPULim: "200m", MemReq: "200Mi", MemLim: "400Mi"},
				},
			},
			patchString: `{"spec":{"containers":[
                                {"name":"c1", "resources":{"requests":{"memory":"300Mi"},"limits":{"memory":"300Mi"}}}
                        ]}}`,
			expected: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "100m", CPULim: "200m", MemReq: "300Mi", MemLim: "300Mi"},
				},
			},
		},
		{
			name: "Burstable QoS pod, one container with cpu & memory requests + limits - decrease CPU requests and increase memory limits",
			containers: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "200m", CPULim: "400m", MemReq: "200Mi", MemLim: "400Mi"},
				},
			},
			patchString: `{"spec":{"containers":[
                                {"name":"c1", "resources":{"requests":{"cpu":"100m"},"limits":{"memory":"500Mi"}}}
                        ]}}`,
			expected: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "100m", CPULim: "400m", MemReq: "200Mi", MemLim: "500Mi"},
				},
			},
		},
		{
			name: "Burstable QoS pod, one container with cpu & memory requests + limits - increase CPU requests and decrease memory limits",
			containers: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "100m", CPULim: "400m", MemReq: "200Mi", MemLim: "500Mi"},
				},
			},
			patchString: `{"spec":{"containers":[
                                {"name":"c1", "resources":{"requests":{"cpu":"200m"},"limits":{"memory":"400Mi"}}}
                        ]}}`,
			expected: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "200m", CPULim: "400m", MemReq: "200Mi", MemLim: "400Mi"},
				},
			},
		},
		{
			name: "Burstable QoS pod, one container with cpu & memory requests + limits - decrease memory requests and increase CPU limits",
			containers: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "100m", CPULim: "200m", MemReq: "200Mi", MemLim: "400Mi"},
				},
			},
			patchString: `{"spec":{"containers":[
                                {"name":"c1", "resources":{"requests":{"memory":"100Mi"},"limits":{"cpu":"300m"}}}
                        ]}}`,
			expected: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "100m", CPULim: "300m", MemReq: "100Mi", MemLim: "400Mi"},
				},
			},
		},
		{
			name: "Burstable QoS pod, one container with cpu & memory requests + limits - increase memory requests and decrease CPU limits",
			containers: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "200m", CPULim: "400m", MemReq: "200Mi", MemLim: "400Mi"},
				},
			},
			patchString: `{"spec":{"containers":[
                                {"name":"c1", "resources":{"requests":{"memory":"300Mi"},"limits":{"cpu":"300m"}}}
                        ]}}`,
			expected: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "200m", CPULim: "300m", MemReq: "300Mi", MemLim: "400Mi"},
				},
			},
		},
		{
			name: "Burstable QoS pod, one container with cpu & memory requests - decrease memory request",
			containers: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "200m", MemReq: "500Mi"},
				},
			},
			patchString: `{"spec":{"containers":[
                                {"name":"c1", "resources":{"requests":{"memory":"400Mi"}}}
                        ]}}`,
			expected: []TestContainerInfo{
				{
					Name:      "c1",
					Resources: &ContainerResources{CPUReq: "200m", MemReq: "400Mi"},
				},
			},
		},
	}

	for idx := range tests {
		tc := tests[idx]
		if vmPod && len(tc.containers) > 1 {
			// Multiple VMs per pod is not supported
			continue
		}
		ginkgo.It(tc.name, func() {
			var tPod, pPod *v1.Pod
			var pErr error
			setDefaultPolicy := func(ci *TestContainerInfo) {
				if ci.CPUPolicy == nil {
					ci.CPUPolicy = &noRestart
				}
				if ci.MemPolicy == nil {
					ci.MemPolicy = &noRestart
				}
			}
			for i := range tc.containers {
				setDefaultPolicy(&tc.containers[i])
			}
			for i := range tc.expected {
				setDefaultPolicy(&tc.expected[i])
			}

			tStamp := strconv.Itoa(time.Now().Nanosecond())
			if vmPod {
				tPod = makeTestVMPod(ns, "testpod", tStamp, tc.containers)
			} else {
				tPod = makeTestPod(ns, "testpod", tStamp, tc.containers)
			}

			ginkgo.By("creating pod")
			pod := podClient.CreateSync(tPod)

			ginkgo.By("verifying the pod is in kubernetes")
			selector := labels.SelectorFromSet(labels.Set(map[string]string{"time": tStamp}))
			options := metav1.ListOptions{LabelSelector: selector.String()}
			pods, err := podClient.List(options)
			framework.ExpectNoError(err, "failed to query for pods")
			gomega.Expect(len(pods.Items) == 1)

			ginkgo.By("verifying initial pod resources, allocations, and policy are as expected")
			verifyPodResources(pod, tc.containers)
			verifyPodAllocations(pod, tc.containers)
			verifyPodResizePolicy(pod, tc.containers)

			ginkgo.By("verifying initial pod status resources and cgroup config are as expected")
			verifyPodStatusResources(pod, tc.containers)
			verifyPodContainersCgroupConfig(pod, tc.containers)

			ginkgo.By("patching pod for resize")
			if vmPod {
				vmPatchString := strings.Replace(tc.patchString, "containers", "virtualMachine", 1)
				vmPatchString = strings.Replace(vmPatchString, "[", "", 1)
				vmPatchString = strings.Replace(vmPatchString, "]", "", 1)
				pPod, pErr = f.ClientSet.CoreV1().Pods(pod.Namespace).Patch(pod.Name,
					types.StrategicMergePatchType, []byte(vmPatchString))
				framework.ExpectNoError(pErr, "failed to patch pod for resize")
			} else {
				pPod, pErr = f.ClientSet.CoreV1().Pods(pod.Namespace).Patch(pod.Name,
					types.StrategicMergePatchType, []byte(tc.patchString))
				framework.ExpectNoError(pErr, "failed to patch pod for resize")
			}

			ginkgo.By("verifying pod patched for resize")
			verifyPodResources(pPod, tc.expected)
			verifyPodAllocations(pPod, tc.containers)

			ginkgo.By("verifying updated cgroup configuration in containers")
			verifyPodContainersCgroupConfig(pPod, tc.expected)

			ginkgo.By("verifying pod resources, allocations, and status after resize")
			waitPodStatusResourcesEqualSpecResources := func() (*v1.Pod, error) {
				for start := time.Now(); time.Since(start) < PollTimeout; time.Sleep(PollInterval) {
					pod, err := podClient.Get(pod.Name, metav1.GetOptions{})
					if err != nil {
						return nil, err
					}
					differs := false
					for idx, w := range pod.Spec.Workloads() {
						var statusResources v1.ResourceRequirements
						if pod.Spec.VirtualMachine != nil {
							statusResources = pod.Status.VirtualMachineStatus.Resources
						} else {
							statusResources = pod.Status.ContainerStatuses[idx].Resources
						}
						if diff.ObjectDiff(w.Resources, statusResources) != "" {
							differs = true
							break
						}
					}
					if differs {
						continue
					}
					return pod, nil
				}
				return nil, fmt.Errorf("timed out waiting for pod spec resources to match status resources")
			}
			rPod, rErr := waitPodStatusResourcesEqualSpecResources()
			framework.ExpectNoError(rErr, "failed to get pod")
			verifyPodResources(rPod, tc.expected)
			verifyPodAllocations(rPod, tc.expected)
			verifyPodStatusResources(rPod, tc.expected)

			ginkgo.By("deleting pod")
			err = framework.DeletePodWithWait(f, f.ClientSet, pod)
			framework.ExpectNoError(err, "failed to delete pod")
		})
	}
}

var _ = ginkgo.Describe("[sig-node] [Arktos-CI] PodInPlaceResizeVM", func() {
	doPodResizeTest(true)
})

var _ = ginkgo.Describe("[sig-node] [Arktos-CI] PodInPlaceResizeContainer", func() {
	doPodResizeTest(false)
})

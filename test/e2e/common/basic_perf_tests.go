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

package common

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"

	"k8s.io/client-go/tools/cache"
	watchtools "k8s.io/client-go/tools/watch"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	imageutils "k8s.io/kubernetes/test/utils/image"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

const CREATE_RETRY_INTERVAL = 500 * time.Millisecond

const WATCH_TIMEOUT = 2 * 60 * time.Minute
const MAX_RETRIES = 100
const NUM_NAMESPACES = 10
const NUM_PODS_PER_NS = 100

func makeTestPod(ns, name, podLabel string) *v1.Pod {
	var testContainers []v1.Container
	cmd := "trap exit TERM; while true; do sleep 1; done"
	tc := v1.Container{
		Name:    "foo",
		Image:   imageutils.GetE2EImage(imageutils.BusyBox),
		Command: []string{"/bin/sh"},
		Args:    []string{"-c", cmd},
	}
	testContainers = append(testContainers, tc)
	pod := &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels: map[string]string{
				"name":     "fooPod",
				"podlabel": podLabel,
			},
		},
		Spec: v1.PodSpec{
			Containers:    testContainers,
			RestartPolicy: v1.RestartPolicyOnFailure,
		},
	}
	return pod
}

var _ = ginkgo.Describe("PodPerf", func() {
	var f []*framework.Framework
	var podClient []*framework.PodClient
	var ns []string
	var pods []*v1.Pod

	numNs := NUM_NAMESPACES
	numPodsPerNs := NUM_PODS_PER_NS
	if os.Getenv("NUM_NAMESPACES") != "" {
		numNs, _ = strconv.Atoi(os.Getenv("NUM_NAMESPACES"))
	}
	if os.Getenv("NUM_PODS_PER_NS") != "" {
		numPodsPerNs, _ = strconv.Atoi(os.Getenv("NUM_PODS_PER_NS"))
	}

	f = make([]*framework.Framework, numNs)
	for i := 0; i < numNs; i++ {
		f[i] = framework.NewDefaultFramework("podperf")
	}
	ginkgo.BeforeEach(func() {
		podClient = make([]*framework.PodClient, numNs)
		ns = make([]string, numNs, numNs)
		for i := 0; i < numNs; i++ {
			podClient[i] = f[i].PodClient()
			ns[i] = f[i].Namespace.Name
		}
	})

	ginkgo.It("PodE2EStartLatency", func() {
		var wgStart, wgGet sync.WaitGroup
		var podStartLatency, podGetApiLatency, podListApiLatency []float64
		createPodAndMeasureTime := func(nsIdx, podIdx int) {
			defer ginkgo.GinkgoRecover()
			podName := fmt.Sprintf("testpod-%d-%d", nsIdx, podIdx)
			podLabel := fmt.Sprintf("testpod-%d", nsIdx)
			tPod := makeTestPod(ns[nsIdx], podName, podLabel)
			idx := nsIdx*numPodsPerNs + podIdx
			ctx, cancel := context.WithTimeout(context.TODO(), WATCH_TIMEOUT)
			defer cancel()
			timeout := int64(WATCH_TIMEOUT)
			fieldSelector := fields.OneTermEqualSelector("metadata.name", podName).String()
			lw := &cache.ListWatch{
				ListFunc: func(options metav1.ListOptions) (object runtime.Object, e error) {
					options.FieldSelector = fieldSelector
					options.TimeoutSeconds = &timeout
					obj, err := f[nsIdx].ClientSet.CoreV1().Pods(ns[nsIdx]).List(options)
					return runtime.Object(obj), err
				},
				WatchFunc: func(options metav1.ListOptions) watch.AggregatedWatchInterface {
					options.FieldSelector = fieldSelector
					options.TimeoutSeconds = &timeout
					return f[nsIdx].ClientSet.CoreV1().Pods(ns[nsIdx]).Watch(options)
				},
			}
			conditionFunc := func(event watch.Event) (bool, error) {
				if p, ok := event.Object.(*v1.Pod); ok {
					return p.Status.Phase == v1.PodRunning, nil
				}
				return false, fmt.Errorf("event object not of type Pod")
			}

			retries := 0
			tStamp1 := time.Now()
			for {
				p, err := f[nsIdx].ClientSet.CoreV1().Pods(ns[nsIdx]).Create(tPod)
				if err == nil {
					pods[idx] = p
					break
				}
				time.Sleep(CREATE_RETRY_INTERVAL)
				retries++
				if retries > MAX_RETRIES {
					framework.ExpectNoError(err, fmt.Sprintf("Failed to create pod %s after %d retries", podName, retries))
					break
				}
			}
			_, err := watchtools.UntilWithSync(ctx, lw, &v1.Pod{}, nil, conditionFunc)
			if err != nil {
				framework.ExpectNoError(err, fmt.Sprintf("Pod %s error waiting for PodRunning: %v", podName, err))
			}
			tStamp2 := time.Now()

			podStartLatency[idx] = float64((tStamp2.UnixNano() - tStamp1.UnixNano())) / 1000000
			wgStart.Done()
		}

		getPodAndMeasureTime := func(nsIdx, podIdx int) {
			idx := nsIdx*numPodsPerNs + podIdx
			tStamp1 := time.Now()
			_, err := podClient[nsIdx].Get(pods[idx].Name, metav1.GetOptions{})
			tStamp2 := time.Now()
			framework.ExpectNoError(err, "failed to get pod")

			podGetApiLatency[idx] = float64((tStamp2.UnixNano() - tStamp1.UnixNano())) / 1000000
			wgGet.Done()
		}

		totalPods := numNs * numPodsPerNs
		ginkgo.By(fmt.Sprintf("Creating %d pods in %d namespaces, with %d pods per namespace", totalPods, numNs, numPodsPerNs))
		wgStart.Add(totalPods)
		wgGet.Add(totalPods)
		podStartLatency = make([]float64, totalPods)
		podGetApiLatency = make([]float64, totalPods)
		podListApiLatency = make([]float64, numNs)
		pods = make([]*v1.Pod, totalPods)
		tsStart := time.Now()
		for i := 0; i < numNs; i++ {
			for j := 0; j < numPodsPerNs; j++ {
				go createPodAndMeasureTime(i, j)
			}
		}
		wgStart.Wait()
		tsDone := time.Now()
		podsPerSecond := float64(numNs*numPodsPerNs) / (float64((tsDone.UnixNano() - tsStart.UnixNano())) / 1000000000)
		e2elog.Logf("All created pods are now running. Pod start throughput for %d pods: %.2f pods per second", totalPods, podsPerSecond)

		for i := 0; i < numNs; i++ {
			for j := 0; j < numPodsPerNs; j++ {
				go getPodAndMeasureTime(i, j)
			}
		}
		wgGet.Wait()
		e2elog.Logf("Get API executed for %d pods.", totalPods)

		for i := 0; i < numNs; i++ {
			podLabel := fmt.Sprintf("testpod-%d", i)
			selector := labels.SelectorFromSet(labels.Set(map[string]string{"podlabel": podLabel}))
			tStamp1 := time.Now()
			listPods, err := podClient[i].List(metav1.ListOptions{LabelSelector: selector.String()})
			tStamp2 := time.Now()
			framework.ExpectNoError(err, "failed to query for pod")
			gomega.Expect(len(listPods.Items)).To(gomega.Equal(numPodsPerNs))
			podListApiLatency[i] = float64((tStamp2.UnixNano() - tStamp1.UnixNano())) / 1000000
			e2elog.Logf("NS %s with %d pods - E2E List Latency: %+v", ns[i], len(listPods.Items), podListApiLatency[i])
		}

		for i := 0; i < numNs; i++ {
			for j := 0; j < numPodsPerNs; j++ {
				idx := i*numPodsPerNs + j
				e2epod.DeletePodOrFail(f[i].ClientSet, ns[i], pods[idx].Name)
			}
		}

		var startLatencySum, getApiLatencySum, listApiLatencySum float64
		sort.Float64s(podStartLatency[:])
		sort.Float64s(podGetApiLatency[:])
		sort.Float64s(podListApiLatency[:])
		for i := 0; i < numNs; i++ {
			listApiLatencySum += podListApiLatency[i]
			for j := 0; j < numPodsPerNs; j++ {
				idx := i*numPodsPerNs + j
				startLatencySum += podStartLatency[idx]
				getApiLatencySum += podGetApiLatency[idx]
				e2elog.Logf("NS: %s, PodIdx: %d. StartLatency: %v GetApiLatency: %v", ns[i], idx, podStartLatency[idx], podGetApiLatency[idx])
			}
		}
		avgStartLatencyMillisec := startLatencySum / float64(totalPods)
		avgGetApiLatencyMillisec := getApiLatencySum / float64(totalPods)
		avgListApiLatencyMillisec := listApiLatencySum / float64(numNs)
		medianStartLatency := podStartLatency[totalPods/2]
		medianGetApiLatency := podGetApiLatency[totalPods/2]
		medianListApiLatency := podListApiLatency[numNs/2]
		if totalPods%2 == 0 {
			medianStartLatency = (medianStartLatency + podStartLatency[(totalPods/2)-1]) / 2
			medianGetApiLatency = (medianGetApiLatency + podGetApiLatency[(totalPods/2)-1]) / 2
		}
		if numNs%2 == 0 {
			medianListApiLatency = (medianListApiLatency + podListApiLatency[(numNs/2)-1]) / 2
		}
		e2elog.Logf("------------------------------------------------------------------")
		e2elog.Logf("Min e2e start latency: %.2f milliseconds", podStartLatency[0])
		e2elog.Logf("Max e2e start latency: %.2f milliseconds", podStartLatency[totalPods-1])
		e2elog.Logf("P75 e2e start latency: %.2f milliseconds", podStartLatency[int(float64(totalPods-1)*75/100)])
		e2elog.Logf("P90 e2e start latency: %.2f milliseconds", podStartLatency[int(float64(totalPods-1)*90/100)])
		e2elog.Logf("P95 e2e start latency: %.2f milliseconds", podStartLatency[int(float64(totalPods-1)*95/100)])
		e2elog.Logf("Median e2e start latency: %.2f milliseconds", medianStartLatency)
		e2elog.Logf("Average e2e start latency for %d pods: %.2f milliseconds", totalPods, avgStartLatencyMillisec)
		e2elog.Logf("Pod start throughput for %d pods: %.2f pods per second", totalPods, podsPerSecond)
		e2elog.Logf("---------------")
		e2elog.Logf("Min e2e Get API latency: %.2f milliseconds", podGetApiLatency[0])
		e2elog.Logf("Max e2e Get API latency: %.2f milliseconds", podGetApiLatency[totalPods-1])
		e2elog.Logf("P75 e2e Get API latency: %.2f milliseconds", podGetApiLatency[int(float64(totalPods-1)*75/100)])
		e2elog.Logf("P90 e2e Get API latency: %.2f milliseconds", podGetApiLatency[int(float64(totalPods-1)*90/100)])
		e2elog.Logf("P95 e2e Get API latency: %.2f milliseconds", podGetApiLatency[int(float64(totalPods-1)*95/100)])
		e2elog.Logf("Median e2e Get API latency: %.2f milliseconds", medianGetApiLatency)
		e2elog.Logf("Average e2e Get API latency for %d pods: %.2f milliseconds", totalPods, avgGetApiLatencyMillisec)
		e2elog.Logf("---------------")
		e2elog.Logf("Min e2e List API latency: %.2f milliseconds", podListApiLatency[0])
		e2elog.Logf("Max e2e List API latency: %.2f milliseconds", podListApiLatency[numNs-1])
		e2elog.Logf("P75 e2e List API latency: %.2f milliseconds", podListApiLatency[int(float64(numNs-1)*75/100)])
		e2elog.Logf("P90 e2e List API latency: %.2f milliseconds", podListApiLatency[int(float64(numNs-1)*90/100)])
		e2elog.Logf("P95 e2e List API latency: %.2f milliseconds", podListApiLatency[int(float64(numNs-1)*95/100)])
		e2elog.Logf("Median e2e List API latency: %.2f milliseconds", medianListApiLatency)
		e2elog.Logf("Average e2e List API latency for %d namespaces: %.2f milliseconds", numNs, avgListApiLatencyMillisec)
		e2elog.Logf("------------------------------------------------------------------")
	})
})

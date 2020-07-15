/*
Copyright 2015 The Kubernetes Authors.
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

package framework

import (
	"sync"
	"testing"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

// ensure the watch delivers the requested and only the requested items.
func consume(t *testing.T, w watch.Interface, rvs []string, done *sync.WaitGroup) {
	defer done.Done()
	for _, rv := range rvs {
		got, ok := <-w.ResultChan()
		if !ok {
			t.Errorf("%#v: unexpected channel close, wanted %v", rvs, rv)
			return
		}
		gotRV := got.Object.(*v1.Pod).ObjectMeta.ResourceVersion
		if e, a := rv, gotRV; e != a {
			t.Errorf("wanted %v, got %v", e, a)
		} else {
			t.Logf("Got %v as expected", gotRV)
		}
	}
	// We should not get anything else.
	got, open := <-w.ResultChan()
	if open {
		t.Errorf("%#v: unwanted object %#v", rvs, got)
	}
}

func TestRCNumber(t *testing.T) {
	pod := func(name string) *v1.Pod {
		return &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		}
	}

	wg := &sync.WaitGroup{}
	wg.Add(3)

	source := NewFakeControllerSource()
	source.Add(pod("foo"))
	source.Modify(pod("foo"))
	source.Modify(pod("foo"))

	aw := source.Watch(metav1.ListOptions{ResourceVersion: "1"})
	if aw.GetErrors() != nil {
		t.Fatalf("Unexpected error: %v", aw.GetErrors())
	}
	go consume(t, aw, []string{"2", "3"}, wg)

	list, err := source.List(metav1.ListOptions{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if e, a := "3", list.(*v1.List).ResourceVersion; e != a {
		t.Errorf("wanted %v, got %v", e, a)
	}

	aw2 := source.Watch(metav1.ListOptions{ResourceVersion: "2"})
	if aw2.GetErrors() != nil {
		t.Fatalf("Unexpected error: %v", aw2.GetErrors())
	}
	go consume(t, aw2, []string{"3"}, wg)

	aw3 := source.Watch(metav1.ListOptions{ResourceVersion: "3"})
	if aw3.GetErrors() != nil {
		t.Fatalf("Unexpected error: %v", aw3.GetErrors())
	}
	go consume(t, aw3, []string{}, wg)
	source.Shutdown()
	wg.Wait()
}
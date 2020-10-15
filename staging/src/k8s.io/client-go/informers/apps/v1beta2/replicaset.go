/*
Copyright The Kubernetes Authors.
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

// Code generated by informer-gen. DO NOT EDIT.

package v1beta2

import (
	time "time"

	appsv1beta2 "k8s.io/api/apps/v1beta2"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	internalinterfaces "k8s.io/client-go/informers/internalinterfaces"
	kubernetes "k8s.io/client-go/kubernetes"
	v1beta2 "k8s.io/client-go/listers/apps/v1beta2"
	cache "k8s.io/client-go/tools/cache"
)

// ReplicaSetInformer provides access to a shared informer and lister for
// ReplicaSets.
type ReplicaSetInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1beta2.ReplicaSetLister
}

type replicaSetInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
	tenant           string
}

// NewReplicaSetInformer constructs a new informer for ReplicaSet type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewReplicaSetInformer(client kubernetes.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredReplicaSetInformer(client, namespace, resyncPeriod, indexers, nil)
}

func NewReplicaSetInformerWithMultiTenancy(client kubernetes.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tenant string) cache.SharedIndexInformer {
	return NewFilteredReplicaSetInformerWithMultiTenancy(client, namespace, resyncPeriod, indexers, nil, tenant)
}

// NewFilteredReplicaSetInformer constructs a new informer for ReplicaSet type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredReplicaSetInformer(client kubernetes.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return NewFilteredReplicaSetInformerWithMultiTenancy(client, namespace, resyncPeriod, indexers, tweakListOptions, "system")
}

func NewFilteredReplicaSetInformerWithMultiTenancy(client kubernetes.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc, tenant string) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.AppsV1beta2().ReplicaSetsWithMultiTenancy(namespace, tenant).List(options)
			},
			WatchFunc: func(options v1.ListOptions) watch.AggregatedWatchInterface {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.AppsV1beta2().ReplicaSetsWithMultiTenancy(namespace, tenant).Watch(options)
			},
		},
		&appsv1beta2.ReplicaSet{},
		resyncPeriod,
		indexers,
	)
}

func (f *replicaSetInformer) defaultInformer(client kubernetes.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredReplicaSetInformerWithMultiTenancy(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions, f.tenant)
}

func (f *replicaSetInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&appsv1beta2.ReplicaSet{}, f.defaultInformer)
}

func (f *replicaSetInformer) Lister() v1beta2.ReplicaSetLister {
	return v1beta2.NewReplicaSetLister(f.Informer().GetIndexer())
}
/*
Copyright 2016 The Kubernetes Authors.
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

package generators

import (
	"fmt"
	"io"
	"strings"

	"k8s.io/gengo/generator"
	"k8s.io/gengo/namer"
	"k8s.io/gengo/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/code-generator/cmd/client-gen/generators/util"
	clientgentypes "k8s.io/code-generator/cmd/client-gen/types"

	"k8s.io/klog"
)

// informerGenerator produces a file of listers for a given GroupVersion and
// type.
type informerGenerator struct {
	generator.DefaultGen
	outputPackage             string
	groupPkgName              string
	groupVersion              clientgentypes.GroupVersion
	groupGoName               string
	typeToGenerate            *types.Type
	imports                   namer.ImportTracker
	clientSetPackage          string
	listersPackage            string
	internalInterfacesPackage string
}

var _ generator.Generator = &informerGenerator{}

func (g *informerGenerator) Filter(c *generator.Context, t *types.Type) bool {
	return t == g.typeToGenerate
}

func (g *informerGenerator) Namers(c *generator.Context) namer.NameSystems {
	return namer.NameSystems{
		"raw": namer.NewRawNamer(g.outputPackage, g.imports),
	}
}

func (g *informerGenerator) Imports(c *generator.Context) (imports []string) {
	imports = append(imports, g.imports.ImportLines()...)
	return
}

func (g *informerGenerator) GenerateType(c *generator.Context, t *types.Type, w io.Writer) error {
	sw := generator.NewSnippetWriter(w, c, "$", "$")

	klog.V(5).Infof("processing type %v", t)

	listerPackage := fmt.Sprintf("%s/%s/%s", g.listersPackage, g.groupPkgName, strings.ToLower(g.groupVersion.Version.NonEmpty()))
	clientSetInterface := c.Universe.Type(types.Name{Package: g.clientSetPackage, Name: "Interface"})
	informerFor := "InformerFor"

	tags, err := util.ParseClientGenTags(append(t.SecondClosestCommentLines, t.CommentLines...))
	if err != nil {
		return err
	}

	m := map[string]interface{}{
		"apiScheme":                       c.Universe.Type(apiScheme),
		"cacheIndexers":                   c.Universe.Type(cacheIndexers),
		"cacheListWatch":                  c.Universe.Type(cacheListWatch),
		"cacheMetaTenantIndexFunc":        c.Universe.Function(cacheMetaTenantIndexFunc),
		"cacheTenantIndex":                c.Universe.Variable(cacheTenantIndex),
		"cacheMetaNamespaceIndexFunc":     c.Universe.Function(cacheMetaNamespaceIndexFunc),
		"cacheNamespaceIndex":             c.Universe.Variable(cacheNamespaceIndex),
		"cacheNewSharedIndexInformer":     c.Universe.Function(cacheNewSharedIndexInformer),
		"cacheSharedIndexInformer":        c.Universe.Type(cacheSharedIndexInformer),
		"clientSetInterface":              clientSetInterface,
		"group":                           namer.IC(g.groupGoName),
		"informerFor":                     informerFor,
		"interfacesTweakListOptionsFunc":  c.Universe.Type(types.Name{Package: g.internalInterfacesPackage, Name: "TweakListOptionsFunc"}),
		"interfacesSharedInformerFactory": c.Universe.Type(types.Name{Package: g.internalInterfacesPackage, Name: "SharedInformerFactory"}),
		"listOptions":                     c.Universe.Type(listOptions),
		"lister":                          c.Universe.Type(types.Name{Package: listerPackage, Name: t.Name.Name + "Lister"}),
		"tenantAll":                       c.Universe.Type(metav1TenantAll),
		"namespaceAll":                    c.Universe.Type(metav1NamespaceAll),
		"namespaced":                      !tags.NonNamespaced && !tags.NonTenanted,
		"tenanted":                        tags.NonNamespaced && !tags.NonTenanted,
		"clusterScoped":                   tags.NonNamespaced && tags.NonTenanted,
		"newLister":                       c.Universe.Function(types.Name{Package: listerPackage, Name: "New" + t.Name.Name + "Lister"}),
		"runtimeObject":                   c.Universe.Type(runtimeObject),
		"timeDuration":                    c.Universe.Type(timeDuration),
		"type":                            t,
		"v1ListOptions":                   c.Universe.Type(v1ListOptions),
		"version":                         namer.IC(g.groupVersion.Version.String()),
		"watchInterface":                  c.Universe.Type(watchInterface),
		"DefaultTenant":                   metav1.TenantAll,
	}

	sw.Do(typeInformerInterface, m)

	switch {
	case m["clusterScoped"]:
		//cluster scope
		sw.Do(typeInformerStruct_ClusterScope, m)
		sw.Do(typeInformerPublicConstructor_ClusterScope, m)
		sw.Do(typeFilteredInformerPublicConstructor_ClusterScope, m)
		sw.Do(typeInformerConstructor_ClusterScope, m)

	case m["tenanted"]:
		// tenant scope
		sw.Do(typeInformerStruct_TenantScope, m)
		sw.Do(typeInformerPublicConstructor_TenantScope, m)
		sw.Do(typeFilteredInformerPublicConstructor_TenantScope, m)
		sw.Do(typeInformerConstructor_TenantScope, m)

	case m["namespaced"]:
		// namespace scope
		sw.Do(typeInformerStruct_NamespaceScope, m)
		sw.Do(typeInformerPublicConstructor_NamespaceScope, m)
		sw.Do(typeFilteredInformerPublicConstructor_NamespaceScope, m)
		sw.Do(typeInformerConstructor_NamespaceScope, m)

	default:
		return fmt.Errorf("The scope of (%s) is not supported, namespaced but not tenanted.", t.Name)
	}

	sw.Do(typeInformerInformer, m)
	sw.Do(typeInformerLister, m)

	return sw.Error()
}

var typeInformerInterface = `
// $.type|public$Informer provides access to a shared informer and lister for
// $.type|publicPlural$.
type $.type|public$Informer interface {
	Informer() $.cacheSharedIndexInformer|raw$
	Lister() $.lister|raw$
}
`

var typeInformerStruct_ClusterScope = `
type $.type|private$Informer struct {
	factory $.interfacesSharedInformerFactory|raw$
	tweakListOptions $.interfacesTweakListOptionsFunc|raw$
}
`

var typeInformerStruct_NamespaceScope = `
type $.type|private$Informer struct {
	factory $.interfacesSharedInformerFactory|raw$
	tweakListOptions $.interfacesTweakListOptionsFunc|raw$
	namespace string
	tenant string
}
`

var typeInformerStruct_TenantScope = `
type $.type|private$Informer struct {
	factory $.interfacesSharedInformerFactory|raw$
	tweakListOptions $.interfacesTweakListOptionsFunc|raw$
	tenant string
}
`

var typeInformerPublicConstructor_ClusterScope = `
// New$.type|public$Informer constructs a new informer for $.type|public$ type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func New$.type|public$Informer(client $.clientSetInterface|raw$, resyncPeriod $.timeDuration|raw$, indexers $.cacheIndexers|raw$) $.cacheSharedIndexInformer|raw$ {
	return NewFiltered$.type|public$Informer(client, resyncPeriod, indexers, nil)
}
`

var typeInformerPublicConstructor_TenantScope = `
// New$.type|public$Informer constructs a new informer for $.type|public$ type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func New$.type|public$Informer(client $.clientSetInterface|raw$, resyncPeriod $.timeDuration|raw$, indexers $.cacheIndexers|raw$) $.cacheSharedIndexInformer|raw$ {
	return NewFiltered$.type|public$Informer(client, resyncPeriod, indexers, nil)
}

func New$.type|public$InformerWithMultiTenancy(client $.clientSetInterface|raw$, resyncPeriod $.timeDuration|raw$, indexers $.cacheIndexers|raw$, tenant string) $.cacheSharedIndexInformer|raw$ {
	return NewFiltered$.type|public$InformerWithMultiTenancy(client, resyncPeriod, indexers, nil, tenant)
}
`

var typeInformerPublicConstructor_NamespaceScope = `
// New$.type|public$Informer constructs a new informer for $.type|public$ type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func New$.type|public$Informer(client $.clientSetInterface|raw$, namespace string, resyncPeriod $.timeDuration|raw$, indexers $.cacheIndexers|raw$) $.cacheSharedIndexInformer|raw$ {
	return NewFiltered$.type|public$Informer(client, namespace, resyncPeriod, indexers, nil)
}

func New$.type|public$InformerWithMultiTenancy(client $.clientSetInterface|raw$, namespace string, resyncPeriod $.timeDuration|raw$, indexers $.cacheIndexers|raw$, tenant string) $.cacheSharedIndexInformer|raw$ {
	return NewFiltered$.type|public$InformerWithMultiTenancy(client, namespace, resyncPeriod, indexers, nil, tenant)
}
`

var typeFilteredInformerPublicConstructor_ClusterScope = `
// NewFiltered$.type|public$Informer constructs a new informer for $.type|public$ type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFiltered$.type|public$Informer(client $.clientSetInterface|raw$, resyncPeriod $.timeDuration|raw$, indexers $.cacheIndexers|raw$, tweakListOptions $.interfacesTweakListOptionsFunc|raw$) $.cacheSharedIndexInformer|raw$ {
	return $.cacheNewSharedIndexInformer|raw$(
		&$.cacheListWatch|raw${
			ListFunc: func(options $.v1ListOptions|raw$) ($.runtimeObject|raw$, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.$.group$$.version$().$.type|publicPlural$().List(options)
			},
			WatchFunc: func(options $.v1ListOptions|raw$) ($.watchInterface|raw$, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.$.group$$.version$().$.type|publicPlural$().Watch(options)
			},
		},
		&$.type|raw${},
		resyncPeriod,
		indexers,
	)
}
`

var typeFilteredInformerPublicConstructor_TenantScope = `
// NewFiltered$.type|public$Informer constructs a new informer for $.type|public$ type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFiltered$.type|public$Informer(client $.clientSetInterface|raw$, resyncPeriod $.timeDuration|raw$, indexers $.cacheIndexers|raw$, tweakListOptions $.interfacesTweakListOptionsFunc|raw$) $.cacheSharedIndexInformer|raw$ {
	return NewFiltered$.type|public$InformerWithMultiTenancy(client, resyncPeriod, indexers, tweakListOptions, "$.DefaultTenant$")
}

func NewFiltered$.type|public$InformerWithMultiTenancy(client $.clientSetInterface|raw$, resyncPeriod $.timeDuration|raw$, indexers $.cacheIndexers|raw$, tweakListOptions $.interfacesTweakListOptionsFunc|raw$, tenant string) $.cacheSharedIndexInformer|raw$ {
	return $.cacheNewSharedIndexInformer|raw$(
		&$.cacheListWatch|raw${
			ListFunc: func(options $.v1ListOptions|raw$) ($.runtimeObject|raw$, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.$.group$$.version$().$.type|publicPlural$WithMultiTenancy(tenant).List(options)
			},
			WatchFunc: func(options $.v1ListOptions|raw$) ($.watchInterface|raw$, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.$.group$$.version$().$.type|publicPlural$WithMultiTenancy(tenant).Watch(options)
			},
		},
		&$.type|raw${},
		resyncPeriod,
		indexers,
	)
}
`

var typeFilteredInformerPublicConstructor_NamespaceScope = `
// NewFiltered$.type|public$Informer constructs a new informer for $.type|public$ type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFiltered$.type|public$Informer(client $.clientSetInterface|raw$, namespace string, resyncPeriod $.timeDuration|raw$, indexers $.cacheIndexers|raw$, tweakListOptions $.interfacesTweakListOptionsFunc|raw$) $.cacheSharedIndexInformer|raw$ {
	return NewFiltered$.type|public$InformerWithMultiTenancy(client, namespace, resyncPeriod, indexers, tweakListOptions, "$.DefaultTenant$") 	
}

func NewFiltered$.type|public$InformerWithMultiTenancy(client $.clientSetInterface|raw$, namespace string, resyncPeriod $.timeDuration|raw$, indexers $.cacheIndexers|raw$, tweakListOptions $.interfacesTweakListOptionsFunc|raw$, tenant string) $.cacheSharedIndexInformer|raw$ {
	return $.cacheNewSharedIndexInformer|raw$(
		&$.cacheListWatch|raw${
			ListFunc: func(options $.v1ListOptions|raw$) ($.runtimeObject|raw$, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.$.group$$.version$().$.type|publicPlural$WithMultiTenancy(namespace, tenant).List(options)
			},
			WatchFunc: func(options $.v1ListOptions|raw$) ($.watchInterface|raw$, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.$.group$$.version$().$.type|publicPlural$WithMultiTenancy(namespace, tenant).Watch(options)
			},
		},
		&$.type|raw${},
		resyncPeriod,
		indexers,
	)
}
`

var typeInformerConstructor_ClusterScope = `
func (f *$.type|private$Informer) defaultInformer(client $.clientSetInterface|raw$, resyncPeriod $.timeDuration|raw$) $.cacheSharedIndexInformer|raw$ {
	return NewFiltered$.type|public$Informer(client, resyncPeriod, $.cacheIndexers|raw${}, f.tweakListOptions)
}
`

var typeInformerConstructor_TenantScope = `
func (f *$.type|private$Informer) defaultInformer(client $.clientSetInterface|raw$, resyncPeriod $.timeDuration|raw$) $.cacheSharedIndexInformer|raw$ {
	return NewFiltered$.type|public$InformerWithMultiTenancy(client, resyncPeriod, $.cacheIndexers|raw${$.cacheTenantIndex|raw$: $.cacheMetaTenantIndexFunc|raw$}, f.tweakListOptions, f.tenant)
}
`

var typeInformerConstructor_NamespaceScope = `
func (f *$.type|private$Informer) defaultInformer(client $.clientSetInterface|raw$, resyncPeriod $.timeDuration|raw$) $.cacheSharedIndexInformer|raw$ {
	return NewFiltered$.type|public$InformerWithMultiTenancy(client, f.namespace, resyncPeriod, $.cacheIndexers|raw${$.cacheNamespaceIndex|raw$: $.cacheMetaNamespaceIndexFunc|raw$}, f.tweakListOptions, f.tenant)
}
`

var typeInformerInformer = `
func (f *$.type|private$Informer) Informer() $.cacheSharedIndexInformer|raw$ {
	return f.factory.$.informerFor$(&$.type|raw${}, f.defaultInformer)
}
`

var typeInformerLister = `
func (f *$.type|private$Informer) Lister() $.lister|raw$ {
	return $.newLister|raw$(f.Informer().GetIndexer())
}
`

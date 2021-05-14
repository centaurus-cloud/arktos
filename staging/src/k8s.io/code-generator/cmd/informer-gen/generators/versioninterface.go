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

	"k8s.io/gengo/generator"
	"k8s.io/gengo/namer"
	"k8s.io/gengo/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/code-generator/cmd/client-gen/generators/util"
)

// versionInterfaceGenerator generates the per-version interface file.
type versionInterfaceGenerator struct {
	generator.DefaultGen
	outputPackage             string
	imports                   namer.ImportTracker
	types                     []*types.Type
	filtered                  bool
	internalInterfacesPackage string
}

var _ generator.Generator = &versionInterfaceGenerator{}

func (g *versionInterfaceGenerator) Filter(c *generator.Context, t *types.Type) bool {
	if !g.filtered {
		g.filtered = true
		return true
	}
	return false
}

func (g *versionInterfaceGenerator) Namers(c *generator.Context) namer.NameSystems {
	return namer.NameSystems{
		"raw": namer.NewRawNamer(g.outputPackage, g.imports),
	}
}

func (g *versionInterfaceGenerator) Imports(c *generator.Context) (imports []string) {
	imports = append(imports, g.imports.ImportLines()...)
	return
}

func (g *versionInterfaceGenerator) GenerateType(c *generator.Context, t *types.Type, w io.Writer) error {
	sw := generator.NewSnippetWriter(w, c, "$", "$")

	m := map[string]interface{}{
		"interfacesTweakListOptionsFunc":  c.Universe.Type(types.Name{Package: g.internalInterfacesPackage, Name: "TweakListOptionsFunc"}),
		"interfacesSharedInformerFactory": c.Universe.Type(types.Name{Package: g.internalInterfacesPackage, Name: "SharedInformerFactory"}),
		"types":                           g.types,
		"DefaultTenant":                   metav1.TenantAll,
	}

	sw.Do(versionTemplate, m)
	for _, typeDef := range g.types {
		tags, err := util.ParseClientGenTags(typeDef.SecondClosestCommentLines)
		if err != nil {
			return err
		}
		m["type"] = typeDef
		switch {
		case tags.NonNamespaced && tags.NonTenanted:
			//cluster scope
			sw.Do(versionFuncTemplate_ClusterScope, m)

		case tags.NonNamespaced && !tags.NonTenanted:
			// tenant scope
			sw.Do(versionFuncTemplate_TenantScope, m)

		case !tags.NonNamespaced && !tags.NonTenanted:
			// namespace scope
			sw.Do(versionFuncTemplate_NamespaceScope, m)

		default:
			return fmt.Errorf("The scope of (%s) is not supported, namespaced but not tenanted.", t.Name)
		}
	}

	return sw.Error()
}

var versionTemplate = `
// Interface provides access to all the informers in this group version.
type Interface interface {
	$range .types -$
		// $.|publicPlural$ returns a $.|public$Informer.
		$.|publicPlural$() $.|public$Informer
	$end$
}

type version struct {
	factory $.interfacesSharedInformerFactory|raw$
	tenant    string
	namespace string
	tweakListOptions $.interfacesTweakListOptionsFunc|raw$
}

// New returns a new Interface.
func New(f $.interfacesSharedInformerFactory|raw$, namespace string, tweakListOptions $.interfacesTweakListOptionsFunc|raw$) Interface {
	return &version{factory: f, tenant: "$.DefaultTenant$", namespace: namespace, tweakListOptions: tweakListOptions}
}

func NewWithMultiTenancy(f $.interfacesSharedInformerFactory|raw$, namespace string, tweakListOptions $.interfacesTweakListOptionsFunc|raw$, tenant string) Interface {

	return &version{factory: f, tenant: tenant, namespace: namespace, tweakListOptions: tweakListOptions}
}
`

var versionFuncTemplate_ClusterScope = `
// $.type|publicPlural$ returns a $.type|public$Informer.
func (v *version) $.type|publicPlural$() $.type|public$Informer {
	return &$.type|private$Informer{factory: v.factory, tweakListOptions: v.tweakListOptions}
}
`

var versionFuncTemplate_TenantScope = `
// $.type|publicPlural$ returns a $.type|public$Informer.
func (v *version) $.type|publicPlural$() $.type|public$Informer {
	return &$.type|private$Informer{factory: v.factory, tenant: v.tenant, tweakListOptions: v.tweakListOptions}
}
`

var versionFuncTemplate_NamespaceScope = `
// $.type|publicPlural$ returns a $.type|public$Informer.
func (v *version) $.type|publicPlural$() $.type|public$Informer {
	return &$.type|private$Informer{factory: v.factory, namespace: v.namespace, tenant: v.tenant, tweakListOptions: v.tweakListOptions}
}
`

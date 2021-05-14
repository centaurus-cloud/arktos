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

package fake

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"k8s.io/gengo/generator"
	"k8s.io/gengo/namer"
	"k8s.io/gengo/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/code-generator/cmd/client-gen/generators/util"
)

// genFakeForGroup produces a file for a group client, e.g. ExtensionsClient for the extension group.
type genFakeForGroup struct {
	generator.DefaultGen
	outputPackage     string
	realClientPackage string
	group             string
	version           string
	groupGoName       string
	// types in this group
	types   []*types.Type
	imports namer.ImportTracker
	// If the genGroup has been called. This generator should only execute once.
	called bool
}

var _ generator.Generator = &genFakeForGroup{}

// We only want to call GenerateType() once per group.
func (g *genFakeForGroup) Filter(c *generator.Context, t *types.Type) bool {
	if !g.called {
		g.called = true
		return true
	}
	return false
}

func (g *genFakeForGroup) Namers(c *generator.Context) namer.NameSystems {
	return namer.NameSystems{
		"raw": namer.NewRawNamer(g.outputPackage, g.imports),
	}
}

func (g *genFakeForGroup) Imports(c *generator.Context) (imports []string) {
	imports = g.imports.ImportLines()
	if len(g.types) != 0 {
		imports = append(imports, fmt.Sprintf("%s \"%s\"", strings.ToLower(filepath.Base(g.realClientPackage)), g.realClientPackage))
	}
	return imports
}

func (g *genFakeForGroup) GenerateType(c *generator.Context, t *types.Type, w io.Writer) error {
	sw := generator.NewSnippetWriter(w, c, "$", "$")

	m := map[string]interface{}{
		"GroupGoName":         g.groupGoName,
		"Version":             namer.IC(g.version),
		"Fake":                c.Universe.Type(types.Name{Package: "k8s.io/client-go/testing", Name: "Fake"}),
		"RESTClientInterface": c.Universe.Type(types.Name{Package: "k8s.io/client-go/rest", Name: "Interface"}),
		"RESTClient":          c.Universe.Type(types.Name{Package: "k8s.io/client-go/rest", Name: "RESTClient"}),
	}

	sw.Do(groupClientTemplate, m)
	for _, t := range g.types {
		tags, err := util.ParseClientGenTags(append(t.SecondClosestCommentLines, t.CommentLines...))
		if err != nil {
			return err
		}
		wrapper := map[string]interface{}{
			"type":              t,
			"GroupGoName":       g.groupGoName,
			"Version":           namer.IC(g.version),
			"realClientPackage": strings.ToLower(filepath.Base(g.realClientPackage)),
			"DefaultTenant":     metav1.TenantNone,
		}

		switch {
		case tags.NonNamespaced && tags.NonTenanted:
			sw.Do(getterImplClusterScoped, wrapper)

		case tags.NonNamespaced && !tags.NonTenanted:
			sw.Do(getterImplTenantScoped, wrapper)

		case !tags.NonNamespaced && !tags.NonTenanted:
			sw.Do(getterImplNamespaceScoped, wrapper)

		default:
			return fmt.Errorf("The scope of (%s) is not supported, namespaced but not tenanted.", t.Name)
		}
	}
	sw.Do(getRESTClient, m)
	sw.Do(getRESTClients, m)
	return sw.Error()
}

var groupClientTemplate = `
type Fake$.GroupGoName$$.Version$ struct {
	*$.Fake|raw$
}
`

var getterImplNamespaceScoped = `
func (c *Fake$.GroupGoName$$.Version$) $.type|publicPlural$(namespace string) $.realClientPackage$.$.type|public$Interface {
	return &Fake$.type|publicPlural${c, namespace, "$.DefaultTenant$"}
}

func (c *Fake$.GroupGoName$$.Version$) $.type|publicPlural$WithMultiTenancy(namespace string, tenant string) $.realClientPackage$.$.type|public$Interface {
	return &Fake$.type|publicPlural${c, namespace, tenant}
}
`

var getterImplTenantScoped = `
func (c *Fake$.GroupGoName$$.Version$) $.type|publicPlural$() $.realClientPackage$.$.type|public$Interface {
	return &Fake$.type|publicPlural${c, "$.DefaultTenant$"}
}

func (c *Fake$.GroupGoName$$.Version$) $.type|publicPlural$WithMultiTenancy(tenant string) $.realClientPackage$.$.type|public$Interface {
	return &Fake$.type|publicPlural${c, tenant}
}
`

var getterImplClusterScoped = `
func (c *Fake$.GroupGoName$$.Version$) $.type|publicPlural$() $.realClientPackage$.$.type|public$Interface {

	return &Fake$.type|publicPlural${c}
}
`

var getRESTClient = `
// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *Fake$.GroupGoName$$.Version$) RESTClient() $.RESTClientInterface|raw$ {
	var ret *$.RESTClient|raw$
	return ret
}
`

var getRESTClients = `
// RESTClients returns all RESTClient that are used to communicate
// with all API servers by this client implementation.
func (c *Fake$.GroupGoName$$.Version$) RESTClients() []$.RESTClientInterface|raw$ {
	var ret *$.RESTClient|raw$
	return []rest.Interface{ret}
}
`

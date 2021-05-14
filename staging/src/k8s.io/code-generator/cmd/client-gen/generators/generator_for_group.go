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

package generators

import (
	"fmt"
	"io"
	"k8s.io/gengo/generator"
	"k8s.io/gengo/namer"
	"k8s.io/gengo/types"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/code-generator/cmd/client-gen/generators/util"
	"k8s.io/code-generator/cmd/client-gen/path"
)

// genGroup produces a file for a group client, e.g. ExtensionsClient for the extension group.
type genGroup struct {
	generator.DefaultGen
	outputPackage string
	group         string
	version       string
	groupGoName   string
	apiPath       string
	// types in this group
	types            []*types.Type
	imports          namer.ImportTracker
	inputPackage     string
	clientsetPackage string
	// If the genGroup has been called. This generator should only execute once.
	called bool
}

var _ generator.Generator = &genGroup{}

// We only want to call GenerateType() once per group.
func (g *genGroup) Filter(c *generator.Context, t *types.Type) bool {
	if !g.called {
		g.called = true
		return true
	}
	return false
}

func (g *genGroup) Namers(c *generator.Context) namer.NameSystems {
	return namer.NameSystems{
		"raw": namer.NewRawNamer(g.outputPackage, g.imports),
	}
}

func (g *genGroup) Imports(c *generator.Context) (imports []string) {
	imports = append(imports, g.imports.ImportLines()...)
	imports = append(imports, filepath.Join(g.clientsetPackage, "scheme"))
	return
}

func (g *genGroup) GenerateType(c *generator.Context, t *types.Type, w io.Writer) error {
	sw := generator.NewSnippetWriter(w, c, "$", "$")

	apiPath := func(group string) string {
		if group == "core" {
			return `"/api"`
		}
		return `"` + g.apiPath + `"`
	}

	groupName := g.group
	if g.group == "core" {
		groupName = ""
	}
	// allow user to define a group name that's different from the one parsed from the directory.
	p := c.Universe.Package(path.Vendorless(g.inputPackage))
	if override := types.ExtractCommentTags("+", p.Comments)["groupName"]; override != nil {
		groupName = override[0]
	}

	m := map[string]interface{}{
		"group":                          g.group,
		"version":                        g.version,
		"groupName":                      groupName,
		"GroupGoName":                    g.groupGoName,
		"Version":                        namer.IC(g.version),
		"types":                          g.types,
		"apiPath":                        apiPath(g.group),
		"klogInfo":                       c.Universe.Function(types.Name{Package: "k8s.io/klog", Name: "Infof"}),
		"klogFatal":                      c.Universe.Function(types.Name{Package: "k8s.io/klog", Name: "Fatalf"}),
		"ranSeed":                        c.Universe.Function(types.Name{Package: "math/rand", Name: "Seed"}),
		"ranIntn":                        c.Universe.Function(types.Name{Package: "math/rand", Name: "Intn"}),
		"schemaGroupVersion":             c.Universe.Type(types.Name{Package: "k8s.io/apimachinery/pkg/runtime/schema", Name: "GroupVersion"}),
		"runtimeAPIVersionInternal":      c.Universe.Variable(types.Name{Package: "k8s.io/apimachinery/pkg/runtime", Name: "APIVersionInternal"}),
		"restConfig":                     c.Universe.Type(types.Name{Package: "k8s.io/client-go/rest", Name: "Config"}),
		"getClientSetsWatcher":           c.Universe.Function(types.Name{Package: "k8s.io/client-go/apiserverupdate", Name: "GetClientSetsWatcher"}),
		"restDefaultKubernetesUserAgent": c.Universe.Function(types.Name{Package: "k8s.io/client-go/rest", Name: "DefaultKubernetesUserAgent"}),
		"restRESTClientInterface":        c.Universe.Type(types.Name{Package: "k8s.io/client-go/rest", Name: "Interface"}),
		"restRESTClientFor":              c.Universe.Function(types.Name{Package: "k8s.io/client-go/rest", Name: "RESTClientFor"}),
		"restRESTClientCopyConfig":       c.Universe.Function(types.Name{Package: "k8s.io/client-go/rest", Name: "CopyConfigs"}),
		"SchemeGroupVersion":             c.Universe.Variable(types.Name{Package: path.Vendorless(g.inputPackage), Name: "SchemeGroupVersion"}),
	}
	sw.Do(groupInterfaceTemplate, m)
	sw.Do(groupClientTemplate, m)
	for _, t := range g.types {
		tags, err := util.ParseClientGenTags(append(t.SecondClosestCommentLines, t.CommentLines...))
		if err != nil {
			return err
		}
		wrapper := map[string]interface{}{
			"type":          t,
			"GroupGoName":   g.groupGoName,
			"Version":       namer.IC(g.version),
			"DefaultTenant": metav1.TenantNone,
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
	sw.Do(newClientForConfigTemplate, m)
	sw.Do(newClientForConfigOrDieTemplate, m)
	sw.Do(newClientForRESTClientTemplate, m)
	if g.version == "" {
		sw.Do(setInternalVersionClientDefaultsTemplate, m)
	} else {
		sw.Do(setClientDefaultsTemplate, m)
	}
	sw.Do(getRESTClient, m)
	sw.Do(getRESTClients, m)

	sw.Do(run, m)
	return sw.Error()
}

var groupInterfaceTemplate = `
type $.GroupGoName$$.Version$Interface interface {
    RESTClient() $.restRESTClientInterface|raw$
    RESTClients() []$.restRESTClientInterface|raw$
    $range .types$ $.|publicPlural$Getter
    $end$
}
`

var groupClientTemplate = `
// $.GroupGoName$$.Version$Client is used to interact with features provided by the $.groupName$ group.
type $.GroupGoName$$.Version$Client struct {
	restClients []$.restRESTClientInterface|raw$
	configs *$.restConfig|raw$
	mux sync.RWMutex
}
`

var getterImplNamespaceScoped = `
func (c *$.GroupGoName$$.Version$Client) $.type|publicPlural$(namespace string) $.type|public$Interface {
	return new$.type|publicPlural$WithMultiTenancy(c, namespace, "$.DefaultTenant$")
}

func (c *$.GroupGoName$$.Version$Client) $.type|publicPlural$WithMultiTenancy(namespace string, tenant string) $.type|public$Interface {
	return new$.type|publicPlural$WithMultiTenancy(c, namespace, tenant)
}
`

var getterImplTenantScoped = `
func (c *$.GroupGoName$$.Version$Client) $.type|publicPlural$() $.type|public$Interface {
	return new$.type|publicPlural$WithMultiTenancy(c, "$.DefaultTenant$")
}

func (c *$.GroupGoName$$.Version$Client) $.type|publicPlural$WithMultiTenancy(tenant string) $.type|public$Interface {
	return new$.type|publicPlural$WithMultiTenancy(c, tenant)
}
`

var getterImplClusterScoped = `
func (c *$.GroupGoName$$.Version$Client) $.type|publicPlural$() $.type|public$Interface {
	return new$.type|publicPlural$(c)
}
`

var newClientForConfigTemplate = `
// NewForConfig creates a new $.GroupGoName$$.Version$Client for the given config.
func NewForConfig(c *$.restConfig|raw$) (*$.GroupGoName$$.Version$Client, error) {
	configs := $.restRESTClientCopyConfig|raw$(c)
	if err := setConfigDefaults(configs); err != nil {
		return nil, err
	}

	clients := make([]rest.Interface, len(configs.GetAllConfigs()))
	for i, config := range configs.GetAllConfigs() {
		client, err := $.restRESTClientFor|raw$(config)
		if err != nil {
			return nil, err
		}
		clients[i] = client
	}

	obj := &$.GroupGoName$$.Version$Client{
		restClients: clients,
		configs:     configs,
	}

	obj.run()

	return obj, nil
}
`

var newClientForConfigOrDieTemplate = `
// NewForConfigOrDie creates a new $.GroupGoName$$.Version$Client for the given config and
// panics if there is an error in the config.
func NewForConfigOrDie(c *$.restConfig|raw$) *$.GroupGoName$$.Version$Client {
	client, err := NewForConfig(c)
	if err != nil {
		panic(err)
	}
	return client
}
`

var getRESTClient = `
// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *$.GroupGoName$$.Version$Client) RESTClient() $.restRESTClientInterface|raw$ {
	if c == nil {
		return nil
	}

	c.mux.RLock()
	defer c.mux.RUnlock()
	max := len(c.restClients)
	if max == 0 {
		return nil
	}
	if max == 1 {
		return c.restClients[0]
	}

	$.ranSeed|raw$(time.Now().UnixNano())
	ran := $.ranIntn|raw$(max)
	return c.restClients[ran]
}
`

var getRESTClients = `
// RESTClients returns all RESTClient that are used to communicate
// with all API servers by this client implementation.
func (c *$.GroupGoName$$.Version$Client) RESTClients() []$.restRESTClientInterface|raw$ {
	if c == nil {
		return nil
	}
	return c.restClients
}
`

var newClientForRESTClientTemplate = `
// New creates a new $.GroupGoName$$.Version$Client for the given RESTClient.
func New(c $.restRESTClientInterface|raw$) *$.GroupGoName$$.Version$Client {
	clients := []rest.Interface{c}
	return &$.GroupGoName$$.Version$Client{restClients: clients}
}
`

var setInternalVersionClientDefaultsTemplate = `
func setConfigDefaults(configs *$.restConfig|raw$) error {
	for _, config := range configs.GetAllConfigs() {
		config.APIPath = $.apiPath$
		if config.UserAgent == "" {
			config.UserAgent = $.restDefaultKubernetesUserAgent|raw$()
		}
		if config.GroupVersion == nil || config.GroupVersion.Group != scheme.Scheme.PrioritizedVersionsForGroup("$.groupName$")[0].Group {
			gv := scheme.Scheme.PrioritizedVersionsForGroup("$.groupName$")[0]
			config.GroupVersion = &gv
		}
		config.NegotiatedSerializer = scheme.Codecs
	
		if config.QPS == 0 {
			config.QPS = 5
		}
		if config.Burst == 0 {
			config.Burst = 10
		}
	}

	return nil
}
`

var setClientDefaultsTemplate = `
func setConfigDefaults(configs *$.restConfig|raw$) error {
	gv := $.SchemeGroupVersion|raw$

	for _, config := range configs.GetAllConfigs() {
		config.GroupVersion =  &gv
		config.APIPath = $.apiPath$
		config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
		
		if config.UserAgent == "" {
			config.UserAgent = $.restDefaultKubernetesUserAgent|raw$()
		}
	}

	return nil
}
`

var run = `
// run watch api server instance updates and recreate connections to new set of api servers
func (c *$.GroupGoName$$.Version$Client) run() {
	go func(c *$.GroupGoName$$.Version$Client) {
		member := c.configs.WatchUpdate()
		watcherForUpdateComplete := $.getClientSetsWatcher|raw$()
		watcherForUpdateComplete.AddWatcher()

		for range member.Read {
			// create new client
			clients := make([]$.restRESTClientInterface|raw$, len(c.configs.GetAllConfigs()))
			for i, config := range c.configs.GetAllConfigs() {
				client, err := $.restRESTClientFor|raw$(config)
				if err != nil {
					$.klogFatal|raw$("Cannot create rest client for [%+v], err %v", config, err)
					return
				}
				clients[i] = client
			}
			c.mux.Lock()
			$.klogInfo|raw$("Reset restClients. length %v -> %v", len(c.restClients), len(clients))
			c.restClients = clients
			c.mux.Unlock()
			watcherForUpdateComplete.NotifyDone()
		}
	}(c)
}
`

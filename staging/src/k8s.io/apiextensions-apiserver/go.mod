// This is a generated file. Do not edit directly.

module k8s.io/apiextensions-apiserver

go 1.12

require (
	github.com/emicklei/go-restful v2.9.5+incompatible
	github.com/globalsign/mgo v0.0.0-20181015135952-eeefdecb41b8 // indirect
	github.com/go-openapi/analysis v0.19.5 // indirect
	github.com/go-openapi/errors v0.17.2 // indirect
	github.com/go-openapi/loads v0.19.4 // indirect
	github.com/go-openapi/runtime v0.17.2 // indirect
	github.com/go-openapi/spec v0.19.3
	github.com/go-openapi/strfmt v0.19.3
	github.com/go-openapi/validate v0.19.5
	github.com/gogo/protobuf v1.3.1
	github.com/google/go-cmp v0.3.0
	github.com/google/gofuzz v1.1.0
	github.com/googleapis/gnostic v0.1.0
	github.com/pborman/uuid v1.2.0
	github.com/prometheus/client_golang v1.1.0
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.4.0
	go.etcd.io/etcd v0.5.0-alpha.5.0.20200224211402-c65a9e2dd1fd
	gopkg.in/yaml.v2 v2.2.8
	k8s.io/api v0.0.0
	k8s.io/apimachinery v0.0.0
	k8s.io/apiserver v0.0.0
	k8s.io/client-go v0.0.0
	k8s.io/code-generator v0.0.0
	k8s.io/component-base v0.0.0
	k8s.io/klog v1.0.0
	k8s.io/kube-openapi v0.0.0-20200121204235-bf4fb3bd569c
	k8s.io/utils v0.0.0-20200324210504-a9aa75ae1b89
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/BurntSushi/toml => github.com/BurntSushi/toml v0.3.0
	github.com/PuerkitoBio/purell => github.com/PuerkitoBio/purell v1.1.0

	github.com/coreos/go-oidc => github.com/coreos/go-oidc v0.0.0-20180117170138-065b426bd416
	github.com/coreos/go-systemd => github.com/coreos/go-systemd v0.0.0-20180511133405-39ca1b05acc7
	github.com/dgrijalva/jwt-go => github.com/dgrijalva/jwt-go v0.0.0-20160705203006-01aeca54ebda
	github.com/elazarl/goproxy => github.com/elazarl/goproxy v0.0.0-20170405201442-c4fc26588b6e
	github.com/emicklei/go-restful => github.com/emicklei/go-restful v0.0.0-20170410110728-ff4f55a20633
	github.com/evanphx/json-patch => github.com/evanphx/json-patch v0.0.0-20190203023257-5858425f7550
	github.com/go-openapi/analysis => github.com/go-openapi/analysis v0.17.2
	github.com/go-openapi/loads => github.com/go-openapi/loads v0.17.2
	github.com/go-openapi/spec => github.com/go-openapi/spec v0.17.2
	github.com/go-openapi/strfmt => github.com/go-openapi/strfmt v0.17.0
	github.com/go-openapi/validate => github.com/go-openapi/validate v0.18.0
	github.com/gogo/protobuf => github.com/gogo/protobuf v0.0.0-20171007142547-342cbe0a0415
	github.com/golang/mock => github.com/golang/mock v0.0.0-20160127222235-bd3c8e81be01
	github.com/golang/protobuf => github.com/golang/protobuf v1.2.0
	github.com/google/gofuzz => github.com/google/gofuzz v0.0.0-20170612174753-24818f796faf
	github.com/google/uuid => github.com/google/uuid v1.0.0
	github.com/googleapis/gnostic => github.com/googleapis/gnostic v0.0.0-20170729233727-0c5108395e2d
	github.com/gorilla/websocket => github.com/gorilla/websocket v0.0.0-20170926233335-4201258b820c
	github.com/grpc-ecosystem/go-grpc-middleware => github.com/grpc-ecosystem/go-grpc-middleware v0.0.0-20190222133341-cfaf5686ec79
	github.com/grpc-ecosystem/go-grpc-prometheus => github.com/grpc-ecosystem/go-grpc-prometheus v0.0.0-20170330212424-2500245aa611
	github.com/grpc-ecosystem/grpc-gateway => github.com/grpc-ecosystem/grpc-gateway v1.3.0
	github.com/hashicorp/golang-lru => github.com/hashicorp/golang-lru v0.5.0
	github.com/jonboulle/clockwork => github.com/jonboulle/clockwork v0.0.0-20141017032234-72f9bd7c4e0c
	github.com/json-iterator/go => github.com/json-iterator/go v0.0.0-20180701071628-ab8a2e0c74be
	github.com/konsorten/go-windows-terminal-sequences => github.com/konsorten/go-windows-terminal-sequences v1.0.1
	github.com/munnerz/goautoneg => github.com/munnerz/goautoneg v0.0.0-20120707110453-a547fc61f48d
	github.com/onsi/ginkgo => github.com/onsi/ginkgo v1.6.0
	github.com/onsi/gomega => github.com/onsi/gomega v0.0.0-20190113212917-5533ce8a0da3
	github.com/pkg/errors => github.com/pkg/errors v0.8.0
	github.com/prometheus/client_golang => github.com/prometheus/client_golang v0.9.2
	github.com/prometheus/client_model => github.com/prometheus/client_model v0.0.0-20180712105110-5c3871d89910
	github.com/prometheus/common => github.com/prometheus/common v0.0.0-20181126121408-4724e9255275
	github.com/sirupsen/logrus => github.com/sirupsen/logrus v1.2.0
	github.com/soheilhy/cmux => github.com/soheilhy/cmux v0.1.3
	github.com/spf13/cobra => github.com/spf13/cobra v0.0.0-20180319062004-c439c4fa0937
	github.com/spf13/pflag => github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify => github.com/stretchr/testify v1.2.2
	github.com/xiang90/probing => github.com/xiang90/probing v0.0.0-20160813154853-07dd2e8dfe18
	go.uber.org/atomic => go.uber.org/atomic v0.0.0-20181018215023-8dc6146f7569
	go.uber.org/multierr => go.uber.org/multierr v0.0.0-20180122172545-ddea229ff1df
	go.uber.org/zap => go.uber.org/zap v0.0.0-20180814183419-67bc79d13d15
	golang.org/x/crypto => golang.org/x/crypto v0.0.0-20181025213731-e84da0312774
	golang.org/x/lint => golang.org/x/lint v0.0.0-20181217174547-8f45f776aaf1
	golang.org/x/net => golang.org/x/net v0.0.0-20190206173232-65e2d4e15006
	golang.org/x/oauth2 => golang.org/x/oauth2 v0.0.0-20190402181905-9f3314589c9a
	golang.org/x/sync => golang.org/x/sync v0.0.0-20181108010431-42b317875d0f
	golang.org/x/sys => golang.org/x/sys v0.0.0-20190209173611-3b5209105503
	golang.org/x/text => golang.org/x/text v0.3.1-0.20181227161524-e6919f6577db
	golang.org/x/time => golang.org/x/time v0.0.0-20161028155119-f51c12702a4d
	golang.org/x/tools => golang.org/x/tools v0.0.0-20190313210603-aa82965741a9
	gonum.org/v1/gonum => gonum.org/v1/gonum v0.0.0-20190331200053-3d26580ed485
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20170731182057-09f6ed296fc6
	google.golang.org/grpc => google.golang.org/grpc v1.23.0
	gopkg.in/check.v1 => gopkg.in/check.v1 v0.0.0-20161208181325-20d25e280405
	gopkg.in/natefinch/lumberjack.v2 => gopkg.in/natefinch/lumberjack.v2 v2.0.0-20150622162204-20b71e5b60d7
	gopkg.in/square/go-jose.v2 => gopkg.in/square/go-jose.v2 v2.0.0-20180411045311-89060dee6a84
	gopkg.in/yaml.v2 => gopkg.in/yaml.v2 v2.2.1
	k8s.io/api => ../api
	k8s.io/apiextensions-apiserver => ../apiextensions-apiserver
	k8s.io/apimachinery => ../apimachinery
	k8s.io/apiserver => ../apiserver
	k8s.io/client-go => ../client-go
	k8s.io/code-generator => ../code-generator
	k8s.io/component-base => ../component-base
	k8s.io/gengo => k8s.io/gengo v0.0.0-20190116091435-f8a0810f38af
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20190228160746-b3a7cee44a30
	k8s.io/utils => k8s.io/utils v0.0.0-20190221042446-c2654d5206da
	sigs.k8s.io/yaml => sigs.k8s.io/yaml v1.1.0
)

module github.com/outscale-dev/osc-bsu-csi-driver

go 1.23.0

replace (
	gopkg.in/yaml.v3 => gopkg.in/yaml.v3 v3.0.1 // for the CVE-2022-28948
	k8s.io/api => k8s.io/api v0.30.5
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.30.5
	k8s.io/apimachinery => k8s.io/apimachinery v0.30.5
	k8s.io/apiserver => k8s.io/apiserver v0.30.5
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.30.5
	k8s.io/client-go => k8s.io/client-go v0.30.5
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.30.5
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.30.5
	k8s.io/code-generator => k8s.io/code-generator v0.30.5
	k8s.io/component-base => k8s.io/component-base v0.31.0-alpha.2 // HACK: Workaround https://github.com/kubernetes/kubernetes/issues/124687 by using alpha tag
	k8s.io/component-helpers => k8s.io/component-helpers v0.30.5
	k8s.io/controller-manager => k8s.io/controller-manager v0.30.5
	k8s.io/cri-api => k8s.io/cri-api v0.30.5
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.30.5
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.30.5
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.30.5
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.30.5
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.30.5
	k8s.io/kubectl => k8s.io/kubectl v0.30.5
	k8s.io/kubelet => k8s.io/kubelet v0.30.5
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.30.5
	k8s.io/metrics => k8s.io/metrics v0.30.5
	k8s.io/mount-utils => k8s.io/mount-utils v0.30.5
	k8s.io/node-api => k8s.io/node-api v0.30.5
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.30.5
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.30.5
	k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.30.5
	k8s.io/sample-controller => k8s.io/sample-controller v0.30.5
	vbom.ml/util => github.com/fvbommel/util v0.0.2
)

require (
	github.com/aws/aws-sdk-go v1.44.203
	github.com/container-storage-interface/spec v1.8.0
	github.com/golang/mock v1.6.0
	github.com/kubernetes-csi/csi-test/v5 v5.1.0
	github.com/kubernetes-csi/external-snapshotter/client/v8 v8.0.0
	github.com/onsi/ginkgo/v2 v2.22.1
	github.com/onsi/gomega v1.36.2
	github.com/outscale/osc-sdk-go/v2 v2.21.0
	github.com/stretchr/testify v1.9.0
	golang.org/x/sys v0.28.0
	google.golang.org/grpc v1.66.2
	google.golang.org/protobuf v1.36.1
	k8s.io/api v0.32.0-alpha.1
	k8s.io/apimachinery v0.32.0-alpha.1
	k8s.io/client-go v1.5.2
	k8s.io/component-base v0.32.0-alpha.1
	k8s.io/klog/v2 v2.130.1
	k8s.io/kubernetes v1.30.5
	k8s.io/mount-utils v0.30.5
	k8s.io/pod-security-admission v0.0.0
	k8s.io/utils v0.0.0-20240711033017-18e509b52bc8
)

require (
	github.com/NYTimes/gziphandler v1.1.1 // indirect
	github.com/antlr/antlr4/runtime/Go/antlr/v4 v4.0.0-20230305170008-8188dc5388df // indirect
	github.com/asaskevich/govalidator v0.0.0-20190424111038-f61b66f89f4a // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/coreos/go-semver v0.3.1 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/distribution/reference v0.5.0 // indirect
	github.com/emicklei/go-restful/v3 v3.11.0 // indirect
	github.com/evanphx/json-patch v5.6.0+incompatible // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/jsonpointer v0.21.0 // indirect
	github.com/go-openapi/jsonreference v0.20.2 // indirect
	github.com/go-openapi/swag v0.23.0 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/cel-go v0.17.8 // indirect
	github.com/google/gnostic-models v0.6.8 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/pprof v0.0.0-20241210010833-40e02aabc2ad // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.20.0 // indirect
	github.com/imdario/mergo v0.3.6 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/moby/sys/mountinfo v0.6.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/selinux v1.11.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_golang v1.19.1 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.55.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/spf13/cobra v1.8.1 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stoewer/go-strcase v1.2.0 // indirect
	go.etcd.io/etcd/api/v3 v3.5.10 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.10 // indirect
	go.etcd.io/etcd/client/v3 v3.5.10 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.42.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.53.0 // indirect
	go.opentelemetry.io/otel v1.28.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.28.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.27.0 // indirect
	go.opentelemetry.io/otel/metric v1.28.0 // indirect
	go.opentelemetry.io/otel/sdk v1.28.0 // indirect
	go.opentelemetry.io/otel/trace v1.28.0 // indirect
	go.opentelemetry.io/proto/otlp v1.3.1 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/crypto v0.31.0 // indirect
	golang.org/x/exp v0.0.0-20240719175910-8a7402abbf56 // indirect
	golang.org/x/net v0.33.0 // indirect
	golang.org/x/oauth2 v0.21.0 // indirect
	golang.org/x/sync v0.10.0 // indirect
	golang.org/x/term v0.27.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	golang.org/x/tools v0.28.0 // indirect
	google.golang.org/genproto v0.0.0-20231012201019-e917dd12ba7a // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20240604185151-ef581f913117 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240701130421-f6361c86f094 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/apiextensions-apiserver v0.0.0 // indirect
	k8s.io/apiserver v0.30.5 // indirect
	k8s.io/cloud-provider v0.25.6 // indirect
	k8s.io/component-helpers v0.30.5 // indirect
	k8s.io/controller-manager v0.30.5 // indirect
	k8s.io/kms v0.30.5 // indirect
	k8s.io/kube-openapi v0.0.0-20240827152857-f7e401e7b4c2 // indirect
	k8s.io/kubectl v0.0.0 // indirect
	k8s.io/kubelet v0.0.0 // indirect
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.29.0 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.4.1 // indirect
	sigs.k8s.io/yaml v1.4.0 // indirect
)

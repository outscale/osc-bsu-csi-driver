module github.com/outscale-dev/osc-bsu-csi-driver

go 1.15

require (
	github.com/antihax/optional v1.0.0
	github.com/aws/aws-sdk-go v1.42.22
	github.com/container-storage-interface/spec v1.3.0
	github.com/elazarl/goproxy v0.0.0-20181111060418-2ce16c963a8a // indirect
	github.com/golang/mock v1.6.0
	github.com/golang/protobuf v1.5.2
	github.com/imdario/mergo v0.3.9 // indirect
	github.com/kubernetes-csi/csi-test v2.0.0+incompatible
	github.com/kubernetes-csi/external-snapshotter/client/v4 v4.2.0
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.16.0
	github.com/outscale/osc-sdk-go/osc v0.0.0-20210609082153-592f65eab394
	golang.org/x/sys v0.0.0-20210510120138-977fb7262007
	google.golang.org/grpc v1.29.0
	k8s.io/api v0.21.5
	k8s.io/apimachinery v0.21.5
	k8s.io/client-go v0.21.5
	k8s.io/component-base v0.21.5
	k8s.io/klog/v2 v2.20.0
	k8s.io/kubernetes v1.21.5
	k8s.io/mount-utils v0.21.5
	k8s.io/utils v0.0.0-20210111153108-fddb29f9d009
)

replace (
	k8s.io/api => k8s.io/api v0.21.5
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.21.5
	k8s.io/apimachinery => k8s.io/apimachinery v0.21.5
	k8s.io/apiserver => k8s.io/apiserver v0.21.5
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.21.5
	k8s.io/client-go => k8s.io/client-go v0.21.5
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.21.5
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.21.5
	k8s.io/code-generator => k8s.io/code-generator v0.21.5
	k8s.io/component-base => k8s.io/component-base v0.21.5
	k8s.io/component-helpers => k8s.io/component-helpers v0.21.5
	k8s.io/controller-manager => k8s.io/controller-manager v0.21.5
	k8s.io/cri-api => k8s.io/cri-api v0.21.5
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.21.5
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.21.5
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.21.5
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.21.5
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.21.5
	k8s.io/kubectl => k8s.io/kubectl v0.21.5
	k8s.io/kubelet => k8s.io/kubelet v0.21.5
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.21.5
	k8s.io/metrics => k8s.io/metrics v0.21.5
	k8s.io/mount-utils => k8s.io/mount-utils v0.21.5
	k8s.io/node-api => k8s.io/node-api v0.21.5
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.21.5
	k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.21.5
	k8s.io/sample-controller => k8s.io/sample-controller v0.21.5
	vbom.ml/util => github.com/fvbommel/util v0.0.0-20160121211510-db5cfe13f5cc
)

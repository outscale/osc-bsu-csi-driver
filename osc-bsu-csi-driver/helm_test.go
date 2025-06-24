package helm_test

import (
	"bufio"
	"errors"
	"io"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/utils/ptr"
)

func getHelmSpecs(t *testing.T, vars ...string) []runtime.Object {
	vars = append(vars, "credentials.create=true", "credentials.accessKey=foo", "credentials.secretKey=bar")
	args := []string{"template", "--debug"}
	if len(vars) > 0 {
		args = append(args, "--set", strings.Join(vars, ","))
	}
	args = append(args, ".")
	cmd := exec.Command("helm", args...)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err, "helm stdout")
	err = cmd.Start()
	require.NoError(t, err, "helm start")
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = rbacv1.AddToScheme(scheme)
	_ = storagev1.AddToScheme(scheme)
	codecs := serializer.NewCodecFactory(scheme)
	decode := codecs.UniversalDeserializer().Decode

	var specs []runtime.Object
	r := yaml.NewYAMLReader(bufio.NewReader(stdout))
	for {
		buf, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err, "read yaml")
		spec, _, err := decode(buf, nil, nil)
		require.NoError(t, err, "decode yaml")
		specs = append(specs, spec)
	}
	err = cmd.Wait()
	require.NoError(t, err, "helm wait")
	return specs
}

func TestHelmTemplate(t *testing.T) {
	t.Run("The chart contains the right objects", func(t *testing.T) {
		specs := getHelmSpecs(t, "enableVolumeResizing=true", "enableVolumeSnapshot=true")
		require.Len(t, specs, 13)
		objs := map[string]int{}
		for _, obj := range specs {
			objs[reflect.TypeOf(obj).String()]++
		}
		assert.Equal(t, map[string]int{
			"*v1.CSIDriver":          1,
			"*v1.ClusterRole":        4,
			"*v1.ClusterRoleBinding": 4,
			"*v1.DaemonSet":          1,
			"*v1.Deployment":         1,
			"*v1.Secret":             1,
			"*v1.ServiceAccount":     1,
		}, objs)
	})
}

func TestHelmTemplate_Deployment(t *testing.T) {
	var getDeployment = func(t *testing.T, vars ...string) *appsv1.Deployment {
		specs := getHelmSpecs(t, vars...)
		for _, obj := range specs {
			if dep, ok := obj.(*appsv1.Deployment); ok {
				return dep
			}
		}
		return nil
	}
	t.Run("The deployment has the right defaults", func(t *testing.T) {
		dep := getDeployment(t, "enableVolumeResizing=true", "enableVolumeSnapshot=true", "region=eu-west2")
		assert.Equal(t, int32(2), *dep.Spec.Replicas)
		require.Len(t, dep.Spec.Template.Spec.Containers, 6)
		manager := dep.Spec.Template.Spec.Containers[0]
		assert.Equal(t, "outscale/osc-bsu-csi-driver:v1.5.2", manager.Image)
		assert.Equal(t, []string{
			"controller",
			"--endpoint=$(CSI_ENDPOINT)",
			"--logtostderr",
			"--v=3",
		}, manager.Args)
		assert.Equal(t, []corev1.EnvVar{
			{Name: "CSI_ENDPOINT", Value: "unix:///var/lib/csi/sockets/pluginproxy/csi.sock"},
			{Name: "OSC_ACCESS_KEY", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "osc-csi-bsu",
					},
					Key:      "access_key",
					Optional: ptr.To(true),
				},
			}},
			{Name: "OSC_SECRET_KEY", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "osc-csi-bsu",
					},
					Key:      "secret_key",
					Optional: ptr.To(true),
				},
			}},
			{Name: "OSC_REGION", Value: "eu-west2"},
			{Name: "MAX_BSU_VOLUMES", Value: "39"},
			{Name: "BACKOFF_DURATION", Value: "750ms"},
			{Name: "BACKOFF_FACTOR", Value: "1.4"},
			{Name: "BACKOFF_STEPS", Value: "3"},
		}, manager.Env)
		assert.Equal(t, corev1.ResourceRequirements{
			Requests: nil,
			Limits:   nil,
		}, manager.Resources)
	})

	t.Run("Resources can be set globally", func(t *testing.T) {
		dep := getDeployment(t,
			"enableVolumeResizing=true", "enableVolumeSnapshot=true",
			"resources.limits.memory=64Mi", "resources.limits.cpu=10m",
			"resources.requests.memory=96Mi", "resources.requests.cpu=20m")
		require.Len(t, dep.Spec.Template.Spec.Containers, 6)
		for _, container := range dep.Spec.Template.Spec.Containers {
			assert.Equal(t, corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					"memory": resource.MustParse("96Mi"),
					"cpu":    resource.MustParse("20m"),
				},
				Limits: corev1.ResourceList{
					"memory": resource.MustParse("64Mi"),
					"cpu":    resource.MustParse("10m"),
				},
			}, container.Resources, container.Name)
		}
	})

	t.Run("Sidecar resources can be set individually", func(t *testing.T) {
		dep := getDeployment(t,
			"enableVolumeResizing=true", "enableVolumeSnapshot=true",

			"sidecars.provisionerImage.resources.limits.memory=65Mi", "sidecars.provisionerImage.resources.limits.cpu=11m",
			"sidecars.provisionerImage.resources.requests.memory=97Mi", "sidecars.provisionerImage.resources.requests.cpu=21m",

			"sidecars.attacherImage.resources.limits.memory=66Mi", "sidecars.attacherImage.resources.limits.cpu=12m",
			"sidecars.attacherImage.resources.requests.memory=98Mi", "sidecars.attacherImage.resources.requests.cpu=22m",

			"sidecars.snapshotterImage.resources.limits.memory=67Mi", "sidecars.snapshotterImage.resources.limits.cpu=13m",
			"sidecars.snapshotterImage.resources.requests.memory=99Mi", "sidecars.snapshotterImage.resources.requests.cpu=23m",

			"sidecars.resizerImage.resources.limits.memory=68Mi", "sidecars.resizerImage.resources.limits.cpu=14m",
			"sidecars.resizerImage.resources.requests.memory=100Mi", "sidecars.resizerImage.resources.requests.cpu=24m",
		)
		require.Len(t, dep.Spec.Template.Spec.Containers, 6)
		container := dep.Spec.Template.Spec.Containers[1]
		assert.Equal(t, "csi-provisioner", container.Name)
		assert.Equal(t, corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				"memory": resource.MustParse("97Mi"),
				"cpu":    resource.MustParse("21m"),
			},
			Limits: corev1.ResourceList{
				"memory": resource.MustParse("65Mi"),
				"cpu":    resource.MustParse("11m"),
			},
		}, container.Resources, container.Name)

		container = dep.Spec.Template.Spec.Containers[2]
		assert.Equal(t, "csi-attacher", container.Name)
		assert.Equal(t, corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				"memory": resource.MustParse("98Mi"),
				"cpu":    resource.MustParse("22m"),
			},
			Limits: corev1.ResourceList{
				"memory": resource.MustParse("66Mi"),
				"cpu":    resource.MustParse("12m"),
			},
		}, container.Resources, container.Name)

		container = dep.Spec.Template.Spec.Containers[3]
		assert.Equal(t, "csi-snapshotter", container.Name)
		assert.Equal(t, corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				"memory": resource.MustParse("99Mi"),
				"cpu":    resource.MustParse("23m"),
			},
			Limits: corev1.ResourceList{
				"memory": resource.MustParse("67Mi"),
				"cpu":    resource.MustParse("13m"),
			},
		}, container.Resources, container.Name)

		container = dep.Spec.Template.Spec.Containers[4]
		assert.Equal(t, "csi-resizer", container.Name)
		assert.Equal(t, corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				"memory": resource.MustParse("100Mi"),
				"cpu":    resource.MustParse("24m"),
			},
			Limits: corev1.ResourceList{
				"memory": resource.MustParse("68Mi"),
				"cpu":    resource.MustParse("14m"),
			},
		}, container.Resources, container.Name)
	})

	t.Run("strategy can be set", func(t *testing.T) {
		dep := getDeployment(t,
			"updateStrategy.type=Recreate",
			"updateStrategy.rollingUpdate.maxSurge=1",
			"updateStrategy.rollingUpdate.maxUnavailable=20%",
		)
		assert.Equal(t, appsv1.DeploymentStrategy{
			Type: appsv1.RecreateDeploymentStrategyType,
			RollingUpdate: &appsv1.RollingUpdateDeployment{
				MaxSurge:       ptr.To(intstr.FromInt(1)),
				MaxUnavailable: ptr.To(intstr.FromString("20%")),
			},
		},
			dep.Spec.Strategy)
	})
}

func TestHelmTemplate_DaemonSet(t *testing.T) {
	var getDaemonSet = func(t *testing.T, vars ...string) *appsv1.DaemonSet {
		specs := getHelmSpecs(t, vars...)
		for _, obj := range specs {
			if dep, ok := obj.(*appsv1.DaemonSet); ok {
				return dep
			}
		}
		return nil
	}
	t.Run("The daemonset has the right defaults", func(t *testing.T) {
		dep := getDaemonSet(t, "enableVolumeResizing=true", "enableVolumeSnapshot=true", "region=eu-west2")
		require.Len(t, dep.Spec.Template.Spec.Containers, 3)
		manager := dep.Spec.Template.Spec.Containers[0]
		assert.Equal(t, "outscale/osc-bsu-csi-driver:v1.5.2", manager.Image)
		assert.Equal(t, []string{
			"node",
			"--endpoint=$(CSI_ENDPOINT)",
			"--logtostderr",
			"--v=3",
		}, manager.Args)
		assert.Equal(t, []corev1.EnvVar{
			{Name: "CSI_ENDPOINT", Value: "unix:/csi/csi.sock"},
			{Name: "BACKOFF_DURATION", Value: "750ms"},
			{Name: "BACKOFF_FACTOR", Value: "1.4"},
			{Name: "BACKOFF_STEPS", Value: "3"},
			{Name: "MAX_BSU_VOLUMES", Value: "39"},
		}, manager.Env)
		assert.Equal(t, corev1.ResourceRequirements{
			Requests: nil,
			Limits:   nil,
		}, manager.Resources)
	})

	t.Run("Resources can be set globally", func(t *testing.T) {
		dep := getDaemonSet(t,
			"enableVolumeResizing=true", "enableVolumeSnapshot=true",
			"resources.limits.memory=64Mi", "resources.limits.cpu=10m",
			"resources.requests.memory=96Mi", "resources.requests.cpu=20m")
		require.Len(t, dep.Spec.Template.Spec.Containers, 3)
		for _, container := range dep.Spec.Template.Spec.Containers {
			assert.Equal(t, corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					"memory": resource.MustParse("96Mi"),
					"cpu":    resource.MustParse("20m"),
				},
				Limits: corev1.ResourceList{
					"memory": resource.MustParse("64Mi"),
					"cpu":    resource.MustParse("10m"),
				},
			}, container.Resources, container.Name)
		}
	})

	t.Run("Resources can be overridden globally", func(t *testing.T) {
		dep := getDaemonSet(t,
			"enableVolumeResizing=true", "enableVolumeSnapshot=true",

			"resources.limits.memory=64Mi", "resources.limits.cpu=10m",
			"resources.requests.memory=96Mi", "resources.requests.cpu=20m",

			"node.resources.limits.memory=65Mi", "node.resources.limits.cpu=11m",
			"node.resources.requests.memory=97Mi", "node.resources.requests.cpu=21m",
		)
		require.Len(t, dep.Spec.Template.Spec.Containers, 3)
		for _, container := range dep.Spec.Template.Spec.Containers {
			assert.Equal(t, corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					"memory": resource.MustParse("97Mi"),
					"cpu":    resource.MustParse("21m"),
				},
				Limits: corev1.ResourceList{
					"memory": resource.MustParse("65Mi"),
					"cpu":    resource.MustParse("11m"),
				},
			}, container.Resources, container.Name)
		}
	})

	t.Run("Additional args can be set", func(t *testing.T) {
		dep := getDaemonSet(t,
			"node.args={--luks-open-flags=--perf-no_read_workqueue,--luks-open-flags=--perf-no_write_workqueue}",
		)
		require.Len(t, dep.Spec.Template.Spec.Containers, 3)
		assert.Equal(t, []string{
			"node", "--endpoint=$(CSI_ENDPOINT)", "--logtostderr", "--v=3", "--luks-open-flags=--perf-no_read_workqueue", "--luks-open-flags=--perf-no_write_workqueue"},
			dep.Spec.Template.Spec.Containers[0].Args)
	})

	t.Run("updateStrategy can be set", func(t *testing.T) {
		dep := getDaemonSet(t,
			"node.updateStrategy.type=OnDelete",
			"node.updateStrategy.rollingUpdate.maxSurge=1",
			"node.updateStrategy.rollingUpdate.maxUnavailable=20%",
		)
		require.Len(t, dep.Spec.Template.Spec.Containers, 3)
		assert.Equal(t, appsv1.DaemonSetUpdateStrategy{
			Type: appsv1.OnDeleteDaemonSetStrategyType,
			RollingUpdate: &appsv1.RollingUpdateDaemonSet{
				MaxSurge:       ptr.To(intstr.FromInt(1)),
				MaxUnavailable: ptr.To(intstr.FromString("20%")),
			},
		},
			dep.Spec.UpdateStrategy)
	})

	t.Run("tolerations can be set", func(t *testing.T) {
		dep := getDaemonSet(t,
			"node.tolerateAllTaints=false",
			"node.tolerations[0].key=foo",
			"node.tolerations[0].operator=Exists",
		)
		require.Len(t, dep.Spec.Template.Spec.Containers, 3)
		assert.Equal(t, []corev1.Toleration{
			{Key: "CriticalAddonsOnly", Operator: corev1.TolerationOpExists},
			{Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoExecute, TolerationSeconds: ptr.To[int64](300)},
			{Key: "foo", Operator: corev1.TolerationOpExists},
		},
			dep.Spec.Template.Spec.Tolerations)
	})

	t.Run("imagePullSecrets can be set", func(t *testing.T) {
		dep := getDaemonSet(t,
			"imagePullSecrets[0].name=regcred",
		)
		require.Len(t, dep.Spec.Template.Spec.Containers, 3)
		assert.Equal(t, []corev1.LocalObjectReference{
			{Name: "regcred"},
		},
			dep.Spec.Template.Spec.ImagePullSecrets)
	})
}

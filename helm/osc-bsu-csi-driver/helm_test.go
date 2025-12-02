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
	vars = append(vars, "cloud.credentials.create=true", "cloud.credentials.accessKey=foo", "cloud.credentials.secretKey=bar")
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
		specs := getHelmSpecs(t, "driver.enableVolumeSnapshot=true")
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
		dep := getDeployment(t, "driver.enableVolumeSnapshot=true", "cloud.region=eu-west2")
		assert.Equal(t, int32(2), *dep.Spec.Replicas)
		require.Len(t, dep.Spec.Template.Spec.Containers, 6)
		manager := dep.Spec.Template.Spec.Containers[0]
		assert.Equal(t, "outscale/osc-bsu-csi-driver:v1.8.0", manager.Image)
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
			{Name: "READ_STATUS_INTERVAL", Value: "2s"},
			{Name: "BACKOFF_DURATION", Value: "1s"},
			{Name: "BACKOFF_FACTOR", Value: "2"},
			{Name: "BACKOFF_STEPS", Value: "5"},
		}, manager.Env)
		assert.Equal(t, corev1.ResourceRequirements{
			Requests: nil,
			Limits:   nil,
		}, manager.Resources)
	})

	t.Run("Controller resources can be set", func(t *testing.T) {
		dep := getDeployment(t,
			"driver.enableVolumeSnapshot=true",
			"controller.resources.limits.memory=64Mi", "controller.resources.limits.cpu=10m",
			"controller.resources.requests.memory=96Mi", "controller.resources.requests.cpu=20m")
		require.Len(t, dep.Spec.Template.Spec.Containers, 6)
		for _, container := range dep.Spec.Template.Spec.Containers {
			if container.Name != "osc-plugin" {
				continue
			}
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

	t.Run("Sidecar resources can be set globally", func(t *testing.T) {
		dep := getDeployment(t,
			"driver.enableVolumeSnapshot=true",
			"sidecars.resources.limits.memory=64Mi", "sidecars.resources.limits.cpu=10m",
			"sidecars.resources.requests.memory=96Mi", "sidecars.resources.requests.cpu=20m")
		require.Len(t, dep.Spec.Template.Spec.Containers, 6)
		for _, container := range dep.Spec.Template.Spec.Containers {
			if container.Name == "osc-plugin" {
				continue
			}
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

	t.Run("imagePullPolicy is set", func(t *testing.T) {
		dep := getDeployment(t,
			"driver.imagePullPolicy=foo",
		)
		for _, container := range dep.Spec.Template.Spec.Containers {
			assert.Equal(t, corev1.PullPolicy("foo"), container.ImagePullPolicy)
		}
	})

	t.Run("Sidecar resources can be set individually", func(t *testing.T) {
		dep := getDeployment(t,
			"driver.enableVolumeSnapshot=true",

			"sidecars.provisioner.resources.limits.memory=65Mi", "sidecars.provisioner.resources.limits.cpu=11m",
			"sidecars.provisioner.resources.requests.memory=97Mi", "sidecars.provisioner.resources.requests.cpu=21m",

			"sidecars.attacher.resources.limits.memory=66Mi", "sidecars.attacher.resources.limits.cpu=12m",
			"sidecars.attacher.resources.requests.memory=98Mi", "sidecars.attacher.resources.requests.cpu=22m",

			"sidecars.snapshotter.resources.limits.memory=67Mi", "sidecars.snapshotter.resources.limits.cpu=13m",
			"sidecars.snapshotter.resources.requests.memory=99Mi", "sidecars.snapshotter.resources.requests.cpu=23m",

			"sidecars.resizer.resources.limits.memory=68Mi", "sidecars.resizer.resources.limits.cpu=14m",
			"sidecars.resizer.resources.requests.memory=100Mi", "sidecars.resizer.resources.requests.cpu=24m",
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

	t.Run("updateStrategy can be set", func(t *testing.T) {
		dep := getDeployment(t,
			"controller.updateStrategy.type=Recreate",
			"controller.updateStrategy.rollingUpdate.maxSurge=1",
			"controller.updateStrategy.rollingUpdate.maxUnavailable=20%",
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

	t.Run("Sidecar kube api access is configured", func(t *testing.T) {
		dep := getDeployment(t,
			"driver.enableVolumeSnapshot=true")
		for _, container := range dep.Spec.Template.Spec.Containers {
			if container.Name == "osc-plugin" || container.Name == "liveness-probe" {
				continue
			}
			assert.Contains(t, container.Args, "--kube-api-qps=20", container.Name)
			assert.Contains(t, container.Args, "--kube-api-burst=100", container.Name)
		}
	})

	t.Run("Sidecar kube api access can be tuned", func(t *testing.T) {
		dep := getDeployment(t,
			"driver.enableVolumeSnapshot=true",
			"sidecars.kubeAPI.QPS=42", "sidecars.kubeAPI.burst=43")
		for _, container := range dep.Spec.Template.Spec.Containers {
			if container.Name == "osc-plugin" || container.Name == "liveness-probe" {
				continue
			}
			assert.Contains(t, container.Args, "--kube-api-qps=42", container.Name)
			assert.Contains(t, container.Args, "--kube-api-burst=43", container.Name)
		}
	})

	t.Run("Sidecar threads are configured", func(t *testing.T) {
		dep := getDeployment(t,
			"driver.enableVolumeSnapshot=true")
		for _, container := range dep.Spec.Template.Spec.Containers {
			if container.Name == "osc-plugin" || container.Name == "liveness-probe" {
				continue
			}
			if container.Name == "csi-resizer" {
				assert.Contains(t, container.Args, "--workers=100", container.Name)
			} else {
				assert.Contains(t, container.Args, "--worker-threads=100", container.Name)
			}
		}
	})

	t.Run("Sidecar threads can be tuned", func(t *testing.T) {
		dep := getDeployment(t,
			"driver.enableVolumeSnapshot=true",
			"sidecars.provisioner.workerThreads=42",
			"sidecars.attacher.workerThreads=43",
			"sidecars.snapshotter.workerThreads=44",
			"sidecars.resizer.workerThreads=45",
		)
		require.Len(t, dep.Spec.Template.Spec.Containers, 6)
		containers := dep.Spec.Template.Spec.Containers
		assert.Contains(t, containers[1].Args, "--worker-threads=42", containers[1].Name)
		assert.Contains(t, containers[2].Args, "--worker-threads=43", containers[2].Name)
		assert.Contains(t, containers[3].Args, "--worker-threads=44", containers[3].Name)
		assert.Contains(t, containers[4].Args, "--workers=45", containers[4].Name)
	})

	t.Run("Sidecar timeout is configured", func(t *testing.T) {
		dep := getDeployment(t,
			"driver.enableVolumeSnapshot=true")
		for _, container := range dep.Spec.Template.Spec.Containers {
			if container.Name == "osc-plugin" || container.Name == "liveness-probe" {
				continue
			}
			assert.Contains(t, container.Args, "--timeout=5m", container.Name)
		}
	})

	t.Run("Sidecar timeout can be tuned", func(t *testing.T) {
		dep := getDeployment(t,
			"driver.enableVolumeSnapshot=true",
			"sidecars.timeout=10m",
		)
		for _, container := range dep.Spec.Template.Spec.Containers {
			if container.Name == "osc-plugin" || container.Name == "liveness-probe" {
				continue
			}
			assert.Contains(t, container.Args, "--timeout=10m", container.Name)
		}
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
		dep := getDaemonSet(t)
		require.Len(t, dep.Spec.Template.Spec.Containers, 3)
		manager := dep.Spec.Template.Spec.Containers[0]
		assert.Equal(t, "outscale/osc-bsu-csi-driver:v1.8.0", manager.Image)
		assert.Equal(t, []string{
			"node",
			"--endpoint=$(CSI_ENDPOINT)",
			"--logtostderr",
			"--v=3",
		}, manager.Args)
		assert.Equal(t, []corev1.EnvVar{
			{Name: "CSI_ENDPOINT", Value: "unix:/csi/csi.sock"},
		}, manager.Env)
		assert.Equal(t, corev1.ResourceRequirements{
			Requests: nil,
			Limits:   nil,
		}, manager.Resources)
		assert.Equal(t, &corev1.SecurityContext{
			ReadOnlyRootFilesystem:   ptr.To(false),
			Privileged:               ptr.To(true),
			AllowPrivilegeEscalation: ptr.To(true),
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeUnconfined,
			},
		}, manager.SecurityContext)
	})

	t.Run("MAX_BSU_VOLUMES can be set", func(t *testing.T) {
		dep := getDaemonSet(t, "driver.maxBsuVolumes=39")
		require.Len(t, dep.Spec.Template.Spec.Containers, 3)
		manager := dep.Spec.Template.Spec.Containers[0]
		assert.Equal(t, "outscale/osc-bsu-csi-driver:v1.8.0", manager.Image)
		assert.Equal(t, []string{
			"node",
			"--endpoint=$(CSI_ENDPOINT)",
			"--logtostderr",
			"--v=3",
		}, manager.Args)
		assert.Equal(t, []corev1.EnvVar{
			{Name: "CSI_ENDPOINT", Value: "unix:/csi/csi.sock"},
			{Name: "MAX_BSU_VOLUMES", Value: "39"},
		}, manager.Env)
		assert.Equal(t, corev1.ResourceRequirements{
			Requests: nil,
			Limits:   nil,
		}, manager.Resources)
	})

	t.Run("Node resources can be set", func(t *testing.T) {
		dep := getDaemonSet(t,
			"driver.enableVolumeSnapshot=true",
			"node.resources.limits.memory=64Mi", "node.resources.limits.cpu=10m",
			"node.resources.requests.memory=96Mi", "node.resources.requests.cpu=20m")
		require.Len(t, dep.Spec.Template.Spec.Containers, 3)
		for _, container := range dep.Spec.Template.Spec.Containers {
			if container.Name != "osc-plugin" {
				continue
			}
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

	t.Run("imagePullPolicy is set", func(t *testing.T) {
		dep := getDaemonSet(t,
			"driver.imagePullPolicy=foo",
		)
		for _, container := range dep.Spec.Template.Spec.Containers {
			assert.Equal(t, corev1.PullPolicy("foo"), container.ImagePullPolicy)
		}
	})

	t.Run("Sidecar resources can be set globally", func(t *testing.T) {
		dep := getDaemonSet(t,
			"driver.enableVolumeSnapshot=true",
			"sidecars.resources.limits.memory=64Mi", "sidecars.resources.limits.cpu=10m",
			"sidecars.resources.requests.memory=96Mi", "sidecars.resources.requests.cpu=20m")
		require.Len(t, dep.Spec.Template.Spec.Containers, 3)
		for _, container := range dep.Spec.Template.Spec.Containers {
			if container.Name == "osc-plugin" {
				continue
			}
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

	t.Run("Additional args can be set", func(t *testing.T) {
		dep := getDaemonSet(t,
			"node.additionalArgs={--luks-open-flags=--perf-no_read_workqueue,--luks-open-flags=--perf-no_write_workqueue}",
		)
		require.Len(t, dep.Spec.Template.Spec.Containers, 3)
		assert.Equal(t, []string{
			"node", "--endpoint=$(CSI_ENDPOINT)", "--logtostderr", "--v=3", "--luks-open-flags=--perf-no_read_workqueue", "--luks-open-flags=--perf-no_write_workqueue"},
			dep.Spec.Template.Spec.Containers[0].Args)
	})

	t.Run("updateStrategy can be set", func(t *testing.T) {
		dep := getDaemonSet(t,
			"node.updateStrategy.type=OnDelete",
			"node.updateStrategy.rollingUpdate.maxSurge=2",
			"node.updateStrategy.rollingUpdate.maxUnavailable=20%",
		)
		require.Len(t, dep.Spec.Template.Spec.Containers, 3)
		assert.Equal(t, appsv1.DaemonSetUpdateStrategy{
			Type: appsv1.OnDeleteDaemonSetStrategyType,
			RollingUpdate: &appsv1.RollingUpdateDaemonSet{
				MaxSurge:       ptr.To(intstr.FromInt(2)),
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

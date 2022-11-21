This documentation explains how to deploy Outscale Cloud Controller Manager.

# Prerequisites

You will need a Kubernetes cluster on 3DS Outscale cloud. The next sections details prerequisites on some cloud resources.

| Plugin Version | Minimal Kubernetes Version | Recommended Kubernetes Version |
| -------------- | -------------------------- | ------------------------------ |
| <= v0.0.10beta | 1.20                       | 1.23                           |

# Configuration

## Cluster Resource Tagging

You must tag some cloud resources with a cluster identifier in order to allow Cloud Controller Manager to identify which resources are part of the cluster.
This includes:
- [VPC and Subnets](https://docs.outscale.com/en/userguide/About-VPCs.html)
- [Instances](https://docs.outscale.com/en/userguide/About-Instances.html)
- [Security Groups](https://docs.outscale.com/en/userguide/About-Security-Groups-(Concepts).html)
- [Route Tables](https://docs.outscale.com/en/userguide/About-Route-Tables.html)

The tag key must be `OscK8sClusterID/my-cluster-id` (adapt `my-cluster-id`) and tag value can be one of the following values:
- `shared`: resource is shared between multiple clusters, and should not be destroyed
- `owned`: the resource is considered owned and managed by the cluster

## Instances Tagging

Additionally, instances must be tagged with their node name.

Tag key is `OscK8sNodeName` and tag value `my-kybernetes-host-name` (`my-kybernetes-host-name` should be the same as `kubernetes.io/hostname` computed).

## Security Groups Tagging

By default, the service controller will automatically create a Security Group for each Load Balancer Unit (LBU) and will attach it to nodes in a VPC setup.

If you want to use a pre-created Security Group to be applied to be attached/associated to the LBU, you must tag it with key `OscK8sMainSG/my-cluster-id` and value `True`.
Note that using LBU has some limitation (see issue [#68](https://github.com/outscale-dev/cloud-provider-osc/issues/68)).

## Networking

Node controller is deployed as a daemon set and will need to access [metadata server](https://docs.outscale.com/en/userguide/Accessing-the-Metadata-and-User-Data-of-an-Instance.html) in order to get information about its node (cpu, memory, addresses, hostname).
To do this, node controller need to be able to access `169.254.169.254/32` through TCP port 80 (http).

If you want more details about network configuration with OpenShift, check [openshift documentation](https://docs.openshift.com/container-platform/4.10/networking/understanding-networking.html).

## Kubelet

Kubelet must be run with `--cloud-provider=external`, (more details in [Cloud Controller Manager Administration](https://kubernetes.io/docs/tasks/administer-cluster/running-cloud-controller/#running-cloud-controller-manager) documentation).

## Configuring Cloud Credentials

Outscale Cloud Controller Manager needs API access in order to create resources (like Load Balancer Units) or fetch some data.

It is recommended to use a specific [Access Key](https://docs.outscale.com/en/userguide/About-Access-Keys.html) and create an [EIM user](https://docs.outscale.com/en/userguide/About-EIM-Users.html) with limited access. Check [EIM policy example](eim-policy.example.json) to apply to such EIM user.

To Avoid commiting any secret, just copy [secrets.example.yml](secrets.example.yml) resource and edit it:
```bash
cp deploy/secrets.example.yml deploy/secrets.yml
```
# Deploy

## Add Secret

Make sure to have kubectl configured and deploy the Secret Resource containing your cloud crendentials:
```
kubectl apply -f deploy/secrets.yaml
```

## Add Cloud Controller Manager

You can then deploy Outscale Cloud Controller Manager using a simple manifest:
```
kubectl apply -f deploy/osc-ccm-manifest.yml
```

Alternatively, you can deploy using Helm:
```
helm upgrade --install --wait --wait-for-jobs k8s-osc-ccm deploy/k8s-osc-ccm --set oscSecretName=osc-secret
```
More [helm options are available](../docs/helm.md)

# Check Deployment

To check if Outscale Cloud Manager has been deployed, check for `osc-cloud-controller-manager`:
```
kubectl get pod -n kube-system -l "app=osc-cloud-controller-manager"
```

You can also deploy a simple application exposed by a Service like [2048 web application](../examples/2048/README.md).
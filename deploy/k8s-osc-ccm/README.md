This documentation provides details regarding Outscale Cloud Manager deployment through helm.
For more details about deployment, please check [deployment documentation](../README.md).

# Helm options

| Parameter          | Description                              | Default                       |
|--------------------|------------------------------------------|-------------------------------|
| `replicaCount`     | k8s replicas                             | `1`                           |
| `image.pullPolicy` | Container pull policy                    | `IfNotPresent`                |
| `image.repository` | Container image to use                   | `outscale/cloud-provider-osc` |
| `image.tag`        | Container image tag to deploy            | `v0.0.10beta`                 |
| `imagePullSecrets` | Specify image pull secrets               | `[]`                          |
| `podLabels`        | Labels for pod                           | `{}`                          |
| `oscSecretName`    | Secret name containing cloud credentials | `osc-secret`                  |
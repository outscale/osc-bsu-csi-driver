[![Project Graduated](https://docs.outscale.com/fr/userguide/_images/Project-Graduated-green.svg)](https://docs.outscale.com/en/userguide/Open-Source-Projects.html) [![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/osc-bsu-csi-driver)](https://artifacthub.io/packages/search?repo=osc-bsu-csi-driver) [![](https://dcbadge.limes.pink/api/server/HUVtY5gT6s?style=flat\&theme=default-inverted)](https://discord.gg/HUVtY5gT6s)

# Outscale Block Storage Unit (BSU) CSI Driver

> We currently maintain two branches: **v1.x** (`master`) and **v0.x** (`OSC-MIGRATION`). If you use **v0.x**, see the migration guide: [Upgrading from v0.x to v1.0.0](#upgrading-from-v0x-to-v100).
> v0.x will continue to receive bug and CVE fixes while in use, but **no new features** will be added.

<p align="center">
  <img alt="Kubernetes Logo" src="https://upload.wikimedia.org/wikipedia/commons/3/39/Kubernetes_logo_without_workmark.svg" width="120px">
</p>

---

## 🌐 Links

* Project repo: [https://github.com/outscale/osc-bsu-csi-driver](https://github.com/outscale/osc-bsu-csi-driver)
* Artifact Hub: [https://artifacthub.io/packages/search?repo=osc-bsu-csi-driver](https://artifacthub.io/packages/search?repo=osc-bsu-csi-driver)
* Join our community on [Discord](https://discord.gg/HUVtY5gT6s)

---

## 📄 Table of Contents

* [Overview](#-overview)
* [Compatibility](#-compatibility)
* [Features](#-features)
* [Kubernetes Usage](#-kubernetes-usage)
* [Configuration (StorageClass Parameters)](#-configuration-storageclass-parameters)
* [Installation](#-installation)
* [Troubleshooting](#-troubleshooting)
* [Upgrade Notes](#-upgrade-notes)
* [Examples](#-examples)
* [Development](#-development)
* [License](#-license)
* [Contributing](#-contributing)

---

## 🧭 Overview

The **Outscale Block Storage Unit (BSU) CSI Driver** implements the Container Storage Interface ([CSI](https://github.com/container-storage-interface/spec/blob/master/spec.md)) for OUTSCALE BSU volumes. It allows container orchestrators (e.g., Kubernetes) to provision, attach, mount, snapshot, and expand BSU volumes.

---

## 🔗 Compatibility

<details>
<summary><strong>CSI Specification Compatibility Matrix</strong></summary>

| Plugin Version  | Compatible CSI Version                                                              | Min K8s | Recommended K8s |
| --------------- | ----------------------------------------------------------------------------------- | ------- | --------------- |
| <= v0.0.14beta  | [v1.3.0](https://github.com/container-storage-interface/spec/releases/tag/v1.3.0)   | 1.16    | 1.22            |
| v0.0.15         | [v1.5.0](https://github.com/container-storage-interface/spec/releases/tag/v1.5.0)   | 1.20    | 1.23            |
| v0.1.0 – v1.3.0 | [v1.5.0](https://github.com/container-storage-interface/spec/releases/tag/v1.5.0)   | 1.20    | 1.23            |
| v0.1.0 – v1.6.x | [v1.8.0](https://github.com/container-storage-interface/spec/releases/tag/v1.8.0)   | 1.20    | 1.30            |
| v1.7.x - v1.8.x | [v1.10.0](https://github.com/container-storage-interface/spec/releases/tag/v1.10.0) | 1.20    | 1.31            |

</details>

---

## ✨ Features

**Implemented CSI gRPCs**

* **Controller**: `CreateVolume`, `DeleteVolume`, `ControllerPublishVolume`, `ControllerUnpublishVolume`, `ControllerGetCapabilities`, `ControllerExpandVolume`, `ControllerModifyVolume`, `ValidateVolumeCapabilities`, `CreateSnapshot`, `DeleteSnapshot`, `ListSnapshots`
* **Node**: `NodeStageVolume`, `NodeUnstageVolume`, `NodePublishVolume`, `NodeUnpublishVolume`, `NodeExpandVolume`, `NodeGetCapabilities`, `NodeGetInfo`, `NodeGetVolumeStats`
* **Identity**: `GetPluginInfo`, `GetPluginCapabilities`, `Probe`

**Not implemented**

* **Controller**: `GetCapacity`, `ListVolumes`, `ControllerGetVolume`
* **Node**: —
* **Identity**: —

**Additional behavior**

* **ControllerExpandVolume**: supports both cold (detached) and hot (attached) volume resize.
* **ControllerModifyVolume**: update `volumeType` and `iopsPerGB` via VolumeAttributeClasses on both cold and hot volumes.

---

## ☸️ Kubernetes Usage

* **Static provisioning**: import existing BSU volumes and mount via PVCs.
* **Dynamic provisioning**: create on-demand volumes via PVCs.
* **Block volumes**: raw block device support for latency-sensitive apps (e.g., MySQL).
* **Volume snapshots**: create and restore from snapshots.
* **Volume encryption**: LUKS + `cryptsetup`.

**Prerequisites**

* A Kubernetes cluster within a compatible version range (see [Compatibility](#-compatibility)).
* Driver access to OUTSCALE APIs using AK/SK credentials (for example via an EIM user with an appropriate policy such as [`example-eim-policy.json`](./example-eim-policy.json)).

---

## 🛠 Configuration (StorageClass Parameters)

These parameters are passed via the StorageClass to `CreateVolumeRequest.parameters`:

<details>
<summary><strong>StorageClass parameters</strong></summary>

| Parameter                                        | Values                   | Default | Description                                                        |
| ------------------------------------------------ | ------------------------ | ------- | ------------------------------------------------------------------ |
| `csi.storage.k8s.io/fstype`                      | `xfs`, `ext2/3/4`        | `ext4`  | Filesystem to format the volume with.                              |
| `type`                                           | `io1`, `gp2`, `standard` | `gp2`   | BSU volume type.                                                   |
| `iopsPerGB`                                      | integer                  | —       | Required when `type=io1`; IOPS per GiB.                            |
| `encrypted`                                      | `true`, `false`          | `false` | Enable LUKS encryption.                                            |
| `csi.storage.k8s.io/node-stage-secret-name`      | string                   | —       | Name of the node-stage secret (see CSI docs).                      |
| `csi.storage.k8s.io/node-stage-secret-namespace` | string                   | —       | Namespace of the node-stage secret (see CSI docs).                 |
| `kmsKeyId`                                       | string                   | —       | Not yet supported.                                                 |
| `luks-cipher`                                    | string                   | —       | LUKS cipher; default depends on `cryptsetup` version.              |
| `luks-hash`                                      | string                   | —       | Password derivation hash; default depends on `cryptsetup` version. |
| `luks-key-size`                                  | string                   | —       | Encryption key size; default depends on `cryptsetup` version.      |

**Notes**

* Parameter names are **case-sensitive**.

</details>

---

## 📦 Installation

See **[Deploy](./deploy.md)** for step-by-step installation (Helm/Manifests) and cluster-specific notes.

**Chart configuration**: see **[Helm Chart Configuration](./helm.md)**.

---

## 🐞 Troubleshooting

Common issues and diagnostics are covered in **[Troubleshooting](./troubleshooting.md)**.

---

## ⬆️ Upgrade Notes

### Upgrading from v0.x to v1.0.0

Follow the **[Migration Process](./migration.md)**.

### Upgrading from v1.6 to v1.7

`maxBsuVolumes` is now computed automatically at driver startup. Manual configuration is usually unnecessary, even when multiple BSU volumes are mounted by the OS.

### Upgrading to Helm chart v2

Many variables have been renamed.

Please refer to the [upgrade guide](../helm/osc-bsu-csi-driver/migration.md).

---

## 💡 Examples

* [Dynamic Provisioning](../examples/kubernetes/dynamic-provisioning)
* [Block Volume](../examples/kubernetes/block-volume)
* [Volume Snapshot](../examples/kubernetes/snapshot)
* [StorageClasses](../examples/kubernetes/storageclass)
* [Volume Resizing](../examples/kubernetes/resizing)
* [Volume Updates (VolumeAttributeClasses)](../examples/kubernetes/volume-attribute-class)
* [Encryption](../examples/kubernetes/encryption/)

---

## 🧪 Development

See **[Development Process](./development.md)**.

---

## 📜 License

© 2025 Outscale SAS

See [LICENSE](./LICENSE) for full details.

---

## 🤝 Contributing

Contributions are welcome!
Please read our **[Contributing Guidelines](./CONTRIBUTING.md)** and **[Code of Conduct](./CODE_OF_CONDUCT.md)** before opening a pull request.

# Copyright 2019 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Docker env
DOCKERFILES := $(shell find . -type f -name '*Dockerfile*' !  -path "./debug/*" )
LINTER_VERSION := v2.12.0

E2E_FOCUS ?= single-az

PKG := github.com/outscale/osc-bsu-csi-driver
IMAGE := outscale/osc-bsu-csi-driver
DEV_REF ?= $(shell git rev-parse HEAD)
IMAGE_TAG ?= ${DEV_REF}-amd64
REGISTRY_IMAGE ?= localhost:$(KIND_REGISTRY_PORT)/osc-bsu-csi-driver
REGISTRY_TAG ?= $(shell date '+%Y%m%d%H%M')
VERSION ?= ${IMAGE_TAG}
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS ?= "-s -w -X ${PKG}/pkg/util.driverVersion=${VERSION} -X ${PKG}/pkg/util.buildDate=${BUILD_DATE}"
GO111MODULE := on
GOPROXY := direct
TRIVY_IMAGE := aquasec/trivy:0.69.3

KIND ?= kind
KIND_CLUSTER ?= csi
KIND_NODE_IMAGE ?= kindest/node:v1.34.8@sha256:02722c2dedddcfc00febf5d27fbeb9b7b2c14294c82109ff4a85d89ac9ba3256
KIND_REGISTRY_PORT ?= 5001

OSC_REGION ?= eu-west-2

.EXPORT_ALL_VARIABLES:


all: help

.PHONY: help
help:
	@echo "help:"
	@echo "  - build              : build binary"
	@echo "  - build-image        : build Docker image"
	@echo "  - dockerlint         : check Dockerfile"
	@echo "  - verify             : check code"
	@echo "  - test               : run all tests"
	@echo "  - test-e2e-single-az : run e2e tests"
	@echo "  - helm-docs          : generate helm doc"


.PHONY: setup-kind
setup-kind: ## Set up a Kind cluster for e2e tests if it does not exist
	@command -v $(KIND) >/dev/null 2>&1 || { \
		echo "Kind is not installed. Please install Kind manually."; \
		exit 1; \
	}
	hack/ensure-dev.sh $(KIND_CLUSTER) $(KIND_NODE_IMAGE)

.PHONY: use-kind
use-kind:
	kubectl config use-context kind-$(KIND_CLUSTER)

.PHONY: credentials
credentials: ## Set Credentials
	octl kube secret --name osc-csi-bsu --namespace kube-system | kubectl apply -f - ||:

.PHONY: setup-dev
setup-dev: setup-kind use-kind credentials

.PHONY: cleanup-dev
cleanup-dev: ## Tear down the Kind cluster used for e2e tests
	@$(KIND) delete cluster --name $(KIND_CLUSTER)

.PHONY: deploy-dev
deploy-dev: build image-tag image-push helm_deploy

.PHONY: build
build:
	goreleaser release --clean --snapshot

.PHONY: verify
verify:
	./hack/verify-all

.PHONY: test
test:
	go test -v -race ./pkg/... ./helm/osc-bsu-csi-driver/...

.PHONY: test-sanity
test-sanity:
	go test -v -race ./tests/sanity/... -ginkgo.v -ginkgo.show-node-events

.PHONY: dockerlint
dockerlint:
	@echo "Lint images =>  $(DOCKERFILES)"
	$(foreach image,$(DOCKERFILES), echo "Lint  ${image} " ; docker run --rm -i hadolint/hadolint:${LINTER_VERSION} hadolint --info DL3008 -t warning - < ${image} || exit 1 ; )

.PHONY: test-e2e
test-e2e:
	go test -v ./tests/e2e -test.timeout 180m -ginkgo.timeout 180m -ginkgo.focus="${E2E_FOCUS}" -ginkgo.v -ginkgo.show-node-events -test.v

bin/mockgen:
	go install github.com/golang/mock/mockgen@latest

.PHONY: mock-generate
mock-generate:
	./hack/update-gomock

.PHONY: trivy-scan
trivy-scan:
	docker pull $(TRIVY_IMAGE)
	docker run --rm \
			-v /var/run/docker.sock:/var/run/docker.sock \
			-v ${PWD}/.trivyignore:/root/.trivyignore \
			-v ${PWD}/.trivyscan/:/root/.trivyscan \
			$(TRIVY_IMAGE) \
			image \
			--exit-code 1 \
			--severity="HIGH,CRITICAL" \
			--ignorefile /root/.trivyignore \
			--skip-files "/bin/osc-bsu-csi-driver" \
			--format sarif -o /root/.trivyscan/report.sarif \
			$(IMAGE):$(IMAGE_TAG)

.PHONY: trivy-ignore-check
trivy-ignore-check:
	@./hack/verify-trivyignore

.PHONY: lint-reuse
lint-reuse:
	docker run --rm --volume $(PWD):/data fsfe/reuse:5.1 lint

image-tag:
	docker tag $(IMAGE):$(IMAGE_TAG) $(REGISTRY_IMAGE):$(REGISTRY_TAG)

image-push:
	docker push $(REGISTRY_IMAGE):$(REGISTRY_TAG)

helm_deploy:
	helm upgrade \
			--install \
			--wait \
			--wait-for-jobs  \
			osc-bsu-csi-driver ./helm/osc-bsu-csi-driver \
			--namespace kube-system \
			--set driver.enableVolumeSnapshot=true \
			--set driver.enableVolumeSnapshotExports=true \
			--set driver.enableVolumeAttributesClass=true \
			--set cloud.region=${OSC_REGION} \
			--set driver.image=$(REGISTRY_IMAGE) \
			--set driver.tag=$(REGISTRY_TAG) \
			--set logs.verbosity=5

helm-docs:
	docker run --rm --volume "$$(pwd):/helm-docs" -u "$$(id -u)" jnorwood/helm-docs:v1.14.2 --output-file ../../docs/helm.md

check-helm-docs:
	./hack/verify-helm-docs

helm-package:
# Copy docs into the archive for ArtfactHub, symlink does not work with helm-git
	cp CHANGELOG-1.X.md helm/osc-bsu-csi-driver/CHANGELOG.md
	cp docs/README.md helm/osc-bsu-csi-driver/
	cp LICENSES/BSD-3-Clause.txt helm/osc-bsu-csi-driver/LICENSE
	helm package helm/osc-bsu-csi-driver -d out-helm
	rm helm/osc-bsu-csi-driver/CHANGELOG.md helm/osc-bsu-csi-driver/README.md helm/osc-bsu-csi-driver/LICENSE

helm-push: helm-package
	helm push out-helm/*.tgz oci://registry-1.docker.io/${DOCKER_USER}

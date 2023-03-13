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
LINTER_VERSION := v1.17.5

E2E_ENV ?= "e2e/osc-bsu-csi-driver:0.0"
E2E_ENV_RUN ?= "e2e-osc-bsu-csi-driver"

PKG := github.com/outscale-dev/osc-bsu-csi-driver
IMAGE := outscale/osc-bsu-csi-driver
IMAGE_TAG ?= $(shell git describe --tags --always --dirty)
VERSION ?= ${IMAGE_TAG}
GIT_COMMIT ?= $(shell git rev-parse HEAD)
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS ?= "-X ${PKG}/pkg/util.driverVersion=${VERSION} -X ${PKG}/pkg/util.gitCommit=${GIT_COMMIT} -X ${PKG}/pkg/util.buildDate=${BUILD_DATE}"
GO111MODULE := on
GOPROXY := direct
TRIVY_IMAGE := aquasec/trivy:0.30.0

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

.PHONY: build
build:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux go build -ldflags ${LDFLAGS} -o bin/osc-bsu-csi-driver ./cmd/

.PHONY: build-image
build-image:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE):$(IMAGE_TAG) .

.PHONY: buildx-image
buildx-image:
	docker buildx build --build-arg VERSION=$(VERSION) --load -t $(IMAGE):$(IMAGE_TAG) . 

.PHONY: verify
verify:
	./hack/verify-all

.PHONY: test
test:
	go test -v -race ./pkg/...

.PHONY: dockerlint
dockerlint:
	@echo "Lint images =>  $(DOCKERFILES)"
	$(foreach image,$(DOCKERFILES), echo "Lint  ${image} " ; docker run --rm -i hadolint/hadolint:${LINTER_VERSION} hadolint - < ${image} || exit 1 ; )

.PHONY: test-e2e-single-az
test-e2e-single-az:
	@echo "test-e2e-single-az"
	docker build  -t $(E2E_ENV) -f ./tests/e2e/docker/Dockerfile_e2eTest .
	docker run --rm \
		-v ${PWD}:/root/osc-bsu-csi-driver \
		-e OSC_ACCESS_KEY=${OSC_ACCESS_KEY} \
		-e OSC_SECRET_KEY=${OSC_SECRET_KEY} \
		-e AWS_AVAILABILITY_ZONES="${OSC_REGION}a" \
		-e OSC_REGION=${OSC_REGION} \
		-e KC="$${KC}" \
		--name $(E2E_ENV_RUN) $(E2E_ENV) tests/e2e/docker/run_e2e_single_az.sh

bin/mockgen: | bin
	go get github.com/golang/mock/mockgen@latest

mockgen: bin/mockgen
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
			--security-checks vuln \
			--format sarif -o /root/.trivyscan/report.sarif \
			$(IMAGE):$(IMAGE_TAG)

.PHONY: trivy-ignore-check
trivy-ignore-check:
	@./hack/verify-trivyignore


REGISTRY_IMAGE ?= $(IMAGE)
REGISTRY_TAG ?= $(IMAGE_TAG)
image-tag:
	docker tag $(IMAGE):$(IMAGE_TAG) $(REGISTRY_IMAGE):$(REGISTRY_TAG)

image-push:
	docker push $(REGISTRY_IMAGE):$(REGISTRY_TAG)

TARGET_IMAGE ?= $(IMAGE)
TARGET_TAG ?= $(IMAGE_TAG)
helm_deploy:
	helm upgrade \
			--install \
			--wait \
			--wait-for-jobs  \
			osc-bsu-csi-driver ./osc-bsu-csi-driver \
			--namespace kube-system \
			--set enableVolumeScheduling=true \
			--set enableVolumeResizing=true \
			--set enableVolumeSnapshot=true \
			--set region=${OSC_REGION} \
			--set image.repository=$(TARGET_IMAGE) \
			--set image.tag=$(TARGET_TAG)

helm-docs:
	docker run --rm --volume "$$(pwd):/helm-docs" -u "$$(id -u)" jnorwood/helm-docs:v1.11.0 --output-file ../docs/helm.md

check-helm-docs:
	./hack/verify-helm-docs

helm-package:
# Copy docs into the archive for ArtfactHub, symlink does not work with helm-git
	cp CHANGELOG-1.X.md osc-bsu-csi-driver/CHANGELOG.md
	cp docs/README.md LICENSE osc-bsu-csi-driver/
	helm package osc-bsu-csi-driver -d out-helm
	rm osc-bsu-csi-driver/CHANGELOG.md osc-bsu-csi-driver/README.md osc-bsu-csi-driver/LICENSE

helm-push: helm-package
	helm push out-helm/*.tgz oci://registry-1.docker.io/${DOCKER_USER}

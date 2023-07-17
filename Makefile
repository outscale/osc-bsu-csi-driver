# Copyright 2018 The Kubernetes Authors.
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
#


# Docker env
DOCKERFILES := $(shell find . -name '*Dockerfile*')
LINTER_VERSION := v1.17.5
BUILD_ENV := "buildenv/cloud-provider-osc:0.0"
BUILD_ENV_RUN := "build-cloud-provider-osc"
DEPLOY_NAME := "k8s-osc-ccm"

SOURCES := $(shell find ./cloud-controller-manager -name '*.go')
GOOS ?= $(shell go env GOOS)
VERSION ?= $(shell git describe --tags --always --dirty)
LDFLAGS   := "-w -s -X 'github.com/outscale-dev/cloud-provider-osc/cloud-controller-manager/utils.version=$(VERSION)'"

# Full log with  -v -x
#GO_ADD_OPTIONS := -v -x

IMAGE = "outscale/cloud-provider-osc"
IMAGE_TAG = "${VERSION}"

export GO111MODULE=on
#GOPATH=$(PWD)

E2E_ENV_RUN := "e2e-cloud-provider"
E2E_ENV := "build-e2e-cloud-provider"

OSC_REGION ?= eu-west-2

TRIVY_IMAGE := aquasec/trivy:0.30.0

.PHONY: help
help:
	@echo "help:"
	@echo "  - build              : build binary"
	@echo "  - build-image        : build Docker image"
	@echo "  - dockerlint         : check Dockerfile"
	@echo "  - verify             : check code"
	@echo "  - test               : run all tests"
	@echo "  - test-e2e           : run e2e tests"
	@echo "  - trivy-scan         : run CVE check on Docker images"
	@echo "  - helm-docs          : generate helm doc"
.PHONY: build
build: $(SOURCES)
	CGO_ENABLED=0 GOOS=$(GOOS) go build $(GO_ADD_OPTIONS) \
		-ldflags $(LDFLAGS) \
		-o osc-cloud-controller-manager \
		cloud-controller-manager/cmd/osc-cloud-controller-manager/main.go

.PHONY: verify
verify: verify-fmt vet

.PHONY: verify-fmt
verify-fmt:
	./hack/verify-gofmt.sh

.PHONY: vet
vet:
	go vet ./...

.PHONY: test
test:
	CGO_ENABLED=1 OSC_ACCESS_KEY=test OSC_SECRET_KEY=test go test -count=1  -v $(shell go list ./cloud-controller-manager/...)


.PHONY: build-image
build-image:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE):$(IMAGE_TAG) .

.PHONY: buildx-image
buildx-image:
	docker buildx build  --build-arg VERSION=$(VERSION) --load -t $(IMAGE):$(IMAGE_TAG) .

.PHONY: dockerlint
dockerlint:
	@echo "Lint images =>  $(DOCKERFILES)"
	$(foreach image,$(DOCKERFILES), echo "Lint  ${image} " ; docker run --rm -i hadolint/hadolint:${LINTER_VERSION} hadolint --ignore DL3006 - < ${image} || exit 1 ; )

.PHONY: test-e2e
test-e2e:
	@echo "e2e-test"
	docker build  -t $(E2E_ENV):latest -f ./tests/e2e/docker/Dockerfile_e2eTest .
	docker run --rm \
		-v ${PWD}:/go/src/cloud-provider-osc \
		-e AWS_ACCESS_KEY_ID=${OSC_ACCESS_KEY} \
		-e AWS_SECRET_ACCESS_KEY=${OSC_SECRET_KEY} \
		-e AWS_DEFAULT_REGION=${OSC_REGION} \
		-e AWS_AVAILABILITY_ZONES="${OSC_REGION}a" \
		-e KC="$${KC}" \
		--name $(E2E_ENV_RUN) $(E2E_ENV):latest tests/e2e/docker/run_e2e_single_az.sh

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
		--wait-for-jobs k8s-osc-ccm \
		--set oscSecretName=osc-secret \
		--set image.repository=$(TARGET_IMAGE) \
		--set image.tag=$(TARGET_TAG) \
		--set image.pullPolicy=Always \
		deploy/k8s-osc-ccm
	kubectl rollout restart ds/osc-cloud-controller-manager -n kube-system
	kubectl rollout status ds/osc-cloud-controller-manager -n kube-system --timeout=30s
	kubectl taint nodes --all node.cloudprovider.kubernetes.io/uninitialized=true:NoSchedule

helm-docs:
	docker run --rm --volume "$$(pwd):/helm-docs" -u "$$(id -u)" jnorwood/helm-docs:v1.11.0 --output-file ../../docs/helm.md

check-helm-docs:
	./hack/verify-helm-docs

helm-manifest:
	@helm template test ./deploy/k8s-osc-ccm/ --values deploy/k8s-osc-ccm/values.yaml > deploy/osc-ccm-manifest.yml

check-helm-manifest:
	./hack/verify-helm-manifest.sh

helm-package:
# Copy docs into the archive for ArtfactHub, symlink does not work with helm-git
	cp docs/CHANGELOG.md docs/README.md LICENSE deploy/k8s-osc-ccm/
	helm package deploy/k8s-osc-ccm -d out-helm
	rm deploy/k8s-osc-ccm/CHANGELOG.md deploy/k8s-osc-ccm/README.md deploy/k8s-osc-ccm/LICENSE 

helm-push: helm-package
	helm push out-helm/*.tgz oci://registry-1.docker.io/${DOCKER_USER}

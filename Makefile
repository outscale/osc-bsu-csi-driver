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
E2E_AZ := "eu-west-2a"
E2E_REGION := "eu-west-2"

PKG := github.com/outscale-dev/osc-bsu-csi-driver
IMAGE := osc/osc-ebs-csi-driver
IMAGE_TAG ?= $(shell git describe --exact-match 2> /dev/null || \
                 git describe --match=$(git rev-parse --short=8 HEAD) --always --dirty --abbrev=8)
VERSION ?= ${IMAGE_TAG}
GIT_COMMIT ?= $(shell git rev-parse HEAD)
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS ?= "-X ${PKG}/pkg/util.driverVersion=${VERSION} -X ${PKG}/pkg/util.gitCommit=${GIT_COMMIT} -X ${PKG}/pkg/util.buildDate=${BUILD_DATE}"
GO111MODULE := on
GOPROXY := direct

TRIVY_IMAGE := aquasec/trivy:0.19.2

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

.PHONY: aws-ebs-csi-driver
build:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux go build -ldflags ${LDFLAGS} -o bin/aws-ebs-csi-driver ./cmd/

.PHONY: build-image
build-image:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE):$(IMAGE_TAG) .

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
		-v ${PWD}:/root/aws-ebs-csi-driver \
		-e OSC_ACCESS_KEY=${OSC_ACCESS_KEY} \
		-e OSC_SECRET_KEY=${OSC_SECRET_KEY} \
		-e AWS_AVAILABILITY_ZONES=${E2E_AZ} \
		-e OSC_REGION=${E2E_REGION} \
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
			$(TRIVY_IMAGE) \
			image \
			--exit-code 1 \
			--severity="HIGH,CRITICAL" \
			--ignorefile /root/.trivyignore \
			$(IMAGE):$(IMAGE_TAG)

.PHONY: trivy-ignore-check
trivy-ignore-check:
	@./hack/verify-trivyignore
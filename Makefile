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
BUILD_ENV := "buildenv/osc-bsu-csi-driver:0.0"
BUILD_ENV_RUN := "build-osc-bsu-csi-driver"

OSC_BSU_WORKDIR := /go/src/github.com/kubernetes-sigs/aws-ebs-csi-driver

E2E_ENV := "e2e/osc-bsu-csi-driver:0.0"
E2E_ENV_RUN := "e2e-osc-bsu-csi-driver"
E2E_AZ := "eu-west-2a"
E2E_REGION := "eu-west-2"

PKG := github.com/kubernetes-sigs/aws-ebs-csi-driver
IMAGE := osc/osc-ebs-csi-driver
IMAGE_TAG := $(shell git describe --exact-match 2> /dev/null || \
                 git describe --match=$(git rev-parse --short=8 HEAD) --always --dirty --abbrev=8)
REGISTRY := registry.kube-system:5001
VERSION := ${IMAGE_TAG}
GIT_COMMIT ?= $(shell git rev-parse HEAD)
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS ?= "-X ${PKG}/pkg/util.driverVersion=${VERSION} -X ${PKG}/pkg/util.gitCommit=${GIT_COMMIT} -X ${PKG}/pkg/util.buildDate=${BUILD_DATE}"
GO111MODULE := on
GOPROXY := direct
RUN_CMD := ""

TRIVY_IMAGE := aquasec/trivy:0.19.2

# Full log with  -v -x
GO_ADD_OPTIONS := -v -x

.EXPORT_ALL_VARIABLES:

.PHONY: aws-ebs-csi-driver
aws-ebs-csi-driver:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux go build $(GO_ADD_OPTIONS) \
		-ldflags ${LDFLAGS}  -o  bin/aws-ebs-csi-driver ./cmd/

.PHONY: debug
debug:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux go build -v -gcflags "-N -l" -ldflags ${LDFLAGS}  -o  bin/aws-ebs-csi-driver ./cmd/

.PHONY: verify
verify:
	./hack/verify-all

.PHONY: test
test:
	go test -v -race ./pkg/...

.PHONY: test-e2e-multi-az
test-e2e-multi-az:
	TESTCONFIG=./tester/multi-az-config.yaml go run tester/cmd/main.go

.PHONY: test-e2e-migration
test-e2e-migration:
	AWS_REGION=us-west-2 AWS_AVAILABILITY_ZONES=us-west-2a GINKGO_FOCUS="\[ebs-csi-migration\]" ./hack/run-e2e-test
	# TODO: enable migration test to use new framework
	#TESTCONFIG=./tester/migration-test-config.yaml go run tester/cmd/main.go

.PHONY: image-release
image-release:
	docker build -t $(IMAGE):$(VERSION) .

.PHONY: image
image:
	docker exec $(BUILD_ENV_RUN) make aws-ebs-csi-driver
	docker build -t $(IMAGE):$(IMAGE_TAG) .

.PHONY: image-tag
image-tag:
	docker tag  $(IMAGE):$(IMAGE_TAG) $(REGISTRY)/$(IMAGE):$(IMAGE_TAG)

.PHONY: push-release
push-release:
	docker push $(IMAGE):$(VERSION)

.PHONY: push
push:
	docker push $(REGISTRY)/$(IMAGE):$(IMAGE_TAG)

.PHONY: dockerlint
dockerlint:
	@echo "Lint images =>  $(DOCKERFILES)"
	$(foreach image,$(DOCKERFILES), echo "Lint  ${image} " ; docker run --rm -i hadolint/hadolint:${LINTER_VERSION} hadolint - < ${image} || exit 1 ; )


.PHONY: build_env
build_env:
	docker stop $(BUILD_ENV_RUN) || true
	docker wait $(BUILD_ENV_RUN) || true
	docker rm -f $(BUILD_ENV_RUN) || true
	docker build  -t $(BUILD_ENV) -f ./debug/Dockerfile_debug .
	mkdir -p ${HOME}/go_cache
	docker run -d -v ${HOME}/go_cache:/go -v $(PWD):$(OSC_BSU_WORKDIR) --rm -it --name $(BUILD_ENV_RUN) $(BUILD_ENV)  bash -l
	bash -c "until [[ `docker inspect -f '{{.State.Running}}' $(BUILD_ENV_RUN)` == "true" ]] ; do  sleep 1 ; done"

.PHONY: test-integration
test-integration:
	./hack/run-integration-test

.PHONY: int-test-image
int-test-image:
	docker build  -t $(IMAGE)-int:latest  . -f ./Dockerfile_IntTest

.PHONY: run-integration-test
run-integration-test:
	./run_int_test.sh

.PHONY: run_int_test
run_int_test:
	./run_int_test.sh

.PHONY: deploy
deploy:
	IMAGE_TAG=$(IMAGE_VERSION) IMAGE_NAME=$(REGISTRY)/$(IMAGE) . ./aws-ebs-csi-driver/deploy.sh

.PHONY: test-e2e-single-az
test-e2e-single-az:
	@echo "test-e2e-single-az"
	docker rm -f $(E2E_ENV_RUN) || true
	docker wait $(E2E_ENV_RUN) || true
	docker build  -t $(E2E_ENV) -f ./tests/e2e/docker/Dockerfile_e2eTest .
	docker run -it -d --rm \
		-v ${PWD}:/root/aws-ebs-csi-driver \
		-v ${HOME}:/e2e-env/ \
		-v /etc/kubectl/:/etc/kubectl/ \
		-e OSC_ACCESS_KEY=${OSC_ACCESS_KEY} \
		-e OSC_SECRET_KEY=${OSC_SECRET_KEY} \
		-e AWS_AVAILABILITY_ZONES=${E2E_AZ} \
		-e OSC_REGION=${E2E_REGION} \
		-e KC="$${KC}" \
		--name $(E2E_ENV_RUN) $(E2E_ENV) bash -l
	docker ps -a
	bash -c "until [[ `docker inspect -f '{{.State.Running}}' $(E2E_ENV_RUN)` == "true" ]] ; do  sleep 1 ; done"
	docker exec $(E2E_ENV_RUN) ./tests/e2e/docker/run_e2e_single_az.sh
	docker stop $(E2E_ENV_RUN) || true
	docker wait $(E2E_ENV_RUN) || true
	docker rm -f $(E2E_ENV_RUN) || true

.PHONY: clean_build_env
clean_build_env:
	docker stop ${BUILD_ENV_RUN} || true
	docker wait ${BUILD_ENV_RUN} || true
	docker rm -f ${BUILD_ENV_RUN} || true
	helm del --purge ${DEPLOY_NAME} --tls || true
	docker stop ${E2E_ENV_RUN} || true
	docker wait ${E2E_ENV_RUN} || true
	docker rm -f ${E2E_ENV_RUN} || true

.PHONY: run_cmd
run_cmd:
	docker exec $(BUILD_ENV_RUN) make $(RUN_CMD)

bin /tmp/helm /tmp/kubeval:
	@mkdir -p $@

bin/helm: | /tmp/helm bin
	@curl -o /tmp/helm/helm.tar.gz -sSL https://get.helm.sh/helm-v3.1.2-linux-amd64.tar.gz
	@tar -zxf /tmp/helm/helm.tar.gz -C bin --strip-components=1
	@rm -rf /tmp/helm/*

bin/kubeval: | /tmp/kubeval bin
	@curl -o /tmp/kubeval/kubeval.tar.gz -sSL https://github.com/instrumenta/kubeval/releases/download/0.15.0/kubeval-linux-amd64.tar.gz
	@tar -zxf /tmp/kubeval/kubeval.tar.gz -C bin kubeval
	@rm -rf /tmp/kubeval/*

bin/mockgen: | bin
	go get github.com/golang/mock/mockgen@latest

bin/golangci-lint: | bin
	echo "Installing golangci-lint..."
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v1.43.0

.PHONY: kubeval
kubeval: bin/kubeval
	bin/kubeval -d deploy/kubernetes/base,deploy/kubernetes/cluster,deploy/kubernetes/overlays -i kustomization.yaml,crd_.+\.yaml,controller_add

mockgen: bin/mockgen
	./hack/update-gomock

.PHONY: trivy-scan
trivy-scan:
	docker pull $(TRIVY_IMAGE)
	docker run --rm \
			-v /var/run/docker.sock:/var/run/docker.sock \
			-v ${HOME}/.trivy_cache:/root/.cache/ \
			-v ${PWD}/.trivyignore:/root/.trivyignore \
			$(TRIVY_IMAGE) \
			image \
			--exit-code 1 \
			--severity="HIGH,CRITICAL" \
			--ignorefile /root/.trivyignore \
			$(IMAGE):$(IMAGE_TAG)


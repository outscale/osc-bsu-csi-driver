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
VERSION ?= $(shell git describe --exact-match | sed -r 's/^v(.*)/\1/' 2> /dev/null || \
                 git describe --match=$(git rev-parse --short=8 HEAD) --always --dirty --abbrev=8)
LDFLAGS   := "-w -s -X 'github.com/outscale-dev/cloud-provider-osc/cloud-controller-manager/utils.version=$(VERSION)'"

# Full log with  -v -x
GO_ADD_OPTIONS := -v -x 

IMAGE = "osc/cloud-provider-osc"
IMAGE_VERSION = "v${VERSION}"

export GO111MODULE=on
#GOPATH=$(PWD)

E2E_ENV_RUN := "e2e-cloud-provider"
E2E_ENV := "build-e2e-cloud-provider"
E2E_AZ := "eu-west-2a"
E2E_REGION := "eu-west-2"

TRIVY_IMAGE := aquasec/trivy:0.19.2

osc-cloud-controller-manager: $(SOURCES)
	CGO_ENABLED=0 GOOS=$(GOOS) go build $(GO_ADD_OPTIONS) \
		-ldflags $(LDFLAGS) \
		-o osc-cloud-controller-manager \
		cloud-controller-manager/cmd/osc-cloud-controller-manager/main.go

.PHONY: printenv
printenv:
	@echo "SOURCES =>  $(SOURCES)"
	@echo "GO111MODULE =>  $(GO111MODULE)"
	@echo "LDFLAGS =>  $(LDFLAGS)"
	@echo "GOOS =>  $(GOOS)"
	@echo "VERSION =>  $(VERSION)"
	@echo "PWD =>  $(PWD)"

.PHONY: check
check: verify-fmt verify-lint vet

.PHONY: test
test:
	CGO_ENABLED=1 go test -count=1  -v $(shell go list ./cloud-controller-manager/...)

.PHONY: verify-fmt
verify-fmt:
	./hack/verify-gofmt.sh

.PHONY: verify-lint
verify-lint:
	which golint 2>&1 >/dev/null || go get golang.org/x/lint/golint
	golint -set_exit_status $(shell go list ./...)

.PHONY: vet
vet:
	go vet ./...

.PHONY: update-fmt
update-fmt:
	./hack/update-gofmt.sh

.PHONY: build-image
build-image:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE):$(IMAGE_VERSION) .

.PHONY: tag-image
tag-image:
	docker tag  $(IMAGE):$(IMAGE_VERSION) $(IMAGE):$(IMAGE_VERSION)

.PHONY: push-release
push-release:
	docker push $(IMAGE):$(IMAGE_VERSION)

.PHONY: build-debug
build-debug:
	docker build  -t osc/cloud-provider-osc:debug -f ./debug/Dockerfile_debug .

.PHONY: run-debug
run-debug:
	docker run -v $(PWD):/go/src/cloud-provider-osc --rm -it osc/cloud-provider-osc:debug bash

.PHONY: dockerlint
dockerlint:
	@echo "Lint images =>  $(DOCKERFILES)"
	$(foreach image,$(DOCKERFILES), echo "Lint  ${image} " ; docker run --rm -i hadolint/hadolint:${LINTER_VERSION} hadolint --ignore DL3006 - < ${image} || exit 1 ; )

.PHONY: build_env
build_env:
	docker stop $(BUILD_ENV_RUN) || true
	docker wait $(BUILD_ENV_RUN) || true
	docker rm -f $(BUILD_ENV_RUN) || true
	docker build  -t $(BUILD_ENV) -f ./debug/Dockerfile_debug .
	docker run -d -v ${HOME}/go_cache:/go -v $(PWD):/go/src/cloud-provider-osc --rm -it --name $(BUILD_ENV_RUN) $(BUILD_ENV)  bash -l
	bash -c "until [[ `docker inspect -f '{{.State.Running}}' $(BUILD_ENV_RUN)` == "true" ]] ; do  sleep 1 ; done"

.PHONY: e2e-test
e2e-test:
	@echo "e2e-test"
	docker stop $(E2E_ENV_RUN) || true
	docker wait $(E2E_ENV_RUN) || true
	docker rm -f $(E2E_ENV_RUN) || true
	docker build  -t $(E2E_ENV) -f ./tests/e2e/docker/Dockerfile_e2eTest .
	docker run -it -d --rm \
		-v ${PWD}:/go/src/cloud-provider-osc \
		-v ${HOME}:/e2e-env/ \
		-e AWS_ACCESS_KEY_ID=${OSC_ACCESS_KEY} \
		-e AWS_SECRET_ACCESS_KEY=${OSC_SECRET_KEY} \
		-e AWS_DEFAULT_REGION=$(E2E_REGION) \
		-e AWS_AVAILABILITY_ZONES=$(E2E_AZ) \
		-e KC="$${KC}" \
		--name $(E2E_ENV_RUN) $(E2E_ENV) bash -l
	docker ps -a
	bash -c "until [[ `docker inspect -f '{{.State.Running}}' $(E2E_ENV_RUN)` == "true" ]] ; do  sleep 1 ; done"
	docker exec $(E2E_ENV_RUN) ./tests/e2e/docker/run_e2e_single_az.sh
	docker stop $(E2E_ENV_RUN) || true
	docker wait $(E2E_ENV_RUN) || true
	docker rm -f $(E2E_ENV_RUN) || true


.PHONY: deploy
deploy:
	IMAGE_TAG=$(IMAGE_VERSION) IMAGE_NAME=$(IMAGE) . ./tests/deploy.sh

.PHONY: clean_build_env
clean_build_env:
	docker stop ${BUILD_ENV_RUN} || true 
	docker wait ${BUILD_ENV_RUN} || true 
	docker rm -f ${BUILD_ENV_RUN} || true
	helm del --purge ${DEPLOY_NAME} --tls || true

.PHONY: run_cmd
run_cmd:
	docker exec $(BUILD_ENV_RUN) make $(RUN_CMD)

.PHONY: trivy-scan
trivy-scan:
	docker pull $(TRIVY_IMAGE)
	docker run --rm \
			-v /var/run/docker.sock:/var/run/docker.sock \
			-v ${HOME}/.trivy_cache:/root/.cache/ \
			$(TRIVY_IMAGE) \
			image \
			--exit-code 1 \
			--severity="HIGH,CRITICAL" \
			$(IMAGE):$(IMAGE_VERSION)

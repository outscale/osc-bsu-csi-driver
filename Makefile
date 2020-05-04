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

SOURCES := $(shell find ./cloud-controller-manager -name '*.go')
GOOS ?= $(shell go env GOOS)
VERSION ?= $(shell git describe --exact-match 2> /dev/null || \
                 git describe --match=$(git rev-parse --short=8 HEAD) --always --dirty --abbrev=8)
LDFLAGS   := "-w -s -X 'main.version=${VERSION}'"

# Full log with  -v -x
GO_ADD_OPTIONS := -v

IMAGE = "osc/cloud-provider-osc"
IMAGE_VERSION = "v${VERSION}"
REGISTRY = "registry.kube-system:5001"

export GO111MODULE=on
#GOPATH=$(PWD)


osc-cloud-controller-manager: $(SOURCES)
	CGO_ENABLED=0 GOOS=$(GOOS) go build $(GO_ADD_OPTIONS)\
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
	go test -count=1 -race -v $(shell go list ./...)

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
	docker build -t $(IMAGE):$(IMAGE_VERSION) .

.PHONY: tag-image
tag-image:
	docker tag  $(IMAGE):$(IMAGE_VERSION) $(REGISTRY)/$(IMAGE):$(IMAGE_VERSION)

.PHONY: push-release
push-release:
	docker push $(REGISTRY)/$(IMAGE):$(IMAGE_VERSION)

.PHONY: build-debug
build-debug:
	docker build  -t osc/cloud-provider-osc:debug -f ./debug/Dockerfile_debug .

.PHONY: run-debug
run-debug:
	docker run -v $(PWD):/go/src/cloud-provider-osc --rm -it osc/cloud-provider-osc:debug bash

.PHONY: dockerlint
dockerlint:
	@echo "Lint images =>  $(DOCKERFILES)"
	$(foreach image,$(DOCKERFILES),docker run --rm -i hadolint/hadolint:${LINTER_VERSION} hadolint --ignore DL3006 - < ${image}; )

.PHONY: build_env
build_env:
	docker stop $(BUILD_ENV_RUN) || true
	docker wait $(BUILD_ENV_RUN) || true
	docker rm -f $(BUILD_ENV_RUN) || true
	docker build  -t $(BUILD_ENV) -f ./debug/Dockerfile_debug .
	docker run -d -v $(PWD):/go/src/cloud-provider-osc --rm -it --name $(BUILD_ENV_RUN) $(BUILD_ENV)  bash -l
	until [[ `docker inspect -f '{{.State.Running}}' $(BUILD_ENV_RUN)` == "true" ]] ; do  sleep 1 ; done

.PHONY: e2e-test
e2e-test:
	. ./tests/e2-tests.sh

.PHONY: deploy
deploy:
	IMAGE_TAG=$(IMAGE_VERSION) IMAGE_NAME=$(REGISTRY)/$(IMAGE) . ./tests/deploy.sh

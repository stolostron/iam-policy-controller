# IBM Confidential
# OCO Source Materials
# (C) Copyright IBM Corporation 2018 All Rights Reserved
# The source code for this program is not published or otherwise divested of its trade secrets, irrespective of what has been deposited with the U.S. Copyright Office.
# Copyright (c) 2020 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

PWD := $(shell pwd)
LOCAL_BIN ?= $(PWD)/bin

# Keep an existing GOPATH, make a private one if it is undefined
GOPATH_DEFAULT := $(PWD)/.go
export GOPATH ?= $(GOPATH_DEFAULT)
GOBIN_DEFAULT := $(GOPATH)/bin
export GOBIN ?= $(GOBIN_DEFAULT)
export PATH := $(LOCAL_BIN):$(GOBIN):$(PATH)
GOARCH = $(shell go env GOARCH)
GOOS = $(shell go env GOOS)

TESTARGS_DEFAULT := -v
export TESTARGS ?= $(TESTARGS_DEFAULT)

# Handle KinD configuration
KIND_NAME ?= test-managed
KIND_NAMESPACE ?= open-cluster-management-agent-addon
KIND_VERSION ?= latest
MANAGED_CLUSTER_NAME ?= managed
WATCH_NAMESPACE ?= $(MANAGED_CLUSTER_NAME)
# Set the Kind version tag
ifeq ($(KIND_VERSION), minimum)
	KIND_ARGS = --image kindest/node:v1.19.16
else ifneq ($(KIND_VERSION), latest)
	KIND_ARGS = --image kindest/node:$(KIND_VERSION)
else
	KIND_ARGS =
endif

# Test coverage threshold
export COVERAGE_MIN ?= 53
COVERAGE_E2E_OUT ?= coverage_e2e.out

# Image URL to use all building/pushing image targets;
# Use your own docker registry and image name for dev/test by overridding the IMG and REGISTRY environment variable.
IMG ?= $(shell cat COMPONENT_NAME 2> /dev/null)
REGISTRY ?= quay.io/stolostron
TAG ?= latest
IMAGE_NAME_AND_VERSION ?= $(REGISTRY)/$(IMG)

# go-get-tool will 'go install' any package $1 and install it to LOCAL_BIN.
define go-get-tool
@set -e ;\
echo "Checking installation of $(1)" ;\
GOBIN=$(LOCAL_BIN) go install $(1)
endef

include build/common/Makefile.common.mk

.PHONY: all
all: test

.PHONY: clean
clean:
	-rm bin/*
	-rm build/_output/bin/*
	-rm coverage*.out
	-rm report*.json
	-rm kubeconfig_managed
	-rm -r vendor/

$(GOBIN):
	@mkdir -p $(GOBIN)

$(LOCAL_BIN):
	@mkdir -p $(LOCAL_BIN)

############################################################
# format section
############################################################

# Run go fmt against code
.PHONY: fmt-dependencies
fmt-dependencies:
	$(call go-get-tool,github.com/daixiang0/gci@v0.10.1)
	$(call go-get-tool,mvdan.cc/gofumpt@v0.5.0)

.PHONY: fmt
fmt: fmt-dependencies
	find . -not \( -path "./.go" -prune \) -name "*.go" | xargs gofmt -s -w
	find . -not \( -path "./.go" -prune \) -name "*.go" | xargs gofumpt -l -w
	find . -not \( -path "./.go" -prune \) -name "*.go" | xargs gci write -s standard -s default -s "prefix($(shell cat go.mod | head -1 | cut -d " " -f 2))"

# Run go vet against code
.PHONY: vet
vet:
	go vet ./...

############################################################
# lint
############################################################

.PHONY: lint-dependencies
lint-dependencies:
	$(call go-get-tool,github.com/golangci/golangci-lint/cmd/golangci-lint@v1.52.2)

.PHONY: lint
lint: lint-dependencies lint-all

############################################################
# build
############################################################

.PHONY: dependencies
dependencies: dependencies-go test-dependencies

.PHONY: dependencies-go
dependencies-go:
	go mod tidy
	go mod download

.PHONY: build
build:
	CGO_ENABLED=1 go build -o ./build/_output/bin/$(IMG) ./

.PHONY: build-images
build-images:
	@docker build -t ${IMAGE_NAME_AND_VERSION} .
	@docker tag ${IMAGE_NAME_AND_VERSION} $(REGISTRY)/$(IMG):$(TAG)

# Run against the configured Kubernetes cluster in ~/.kube/config
.PHONY: run
run: generate fmt vet
	WATCH_NAMESPACE=$(WATCH_NAMESPACE) go run ./main.go --leader-elect=false

############################################################
# Generate manifests
############################################################
CONTROLLER_GEN = $(LOCAL_BIN)/controller-gen
KUSTOMIZE = $(LOCAL_BIN)/kustomize
CRD_OPTIONS ?= "crd:trivialVersions=true,preserveUnknownFields=false"

.PHONY: manifests
manifests: controller-gen kustomize
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=iam-policy-controller paths="./..." output:crd:artifacts:config=deploy/crds/kustomize output:rbac:artifacts:config=deploy/rbac
	$(KUSTOMIZE) build deploy/crds/kustomize > deploy/crds/policy.open-cluster-management.io_iampolicies.yaml

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: generate-operator-yaml
generate-operator-yaml: kustomize manifests
	$(KUSTOMIZE) build deploy/manager > deploy/operator.yaml

.PHONY: controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-get-tool,sigs.k8s.io/controller-tools/cmd/controller-gen@v0.6.1)

.PHONY: kustomize
kustomize: ## Download kustomize locally if necessary.
	$(call go-get-tool,sigs.k8s.io/kustomize/kustomize/v5@v5.0.1)

############################################################
# deploy
############################################################

# Install CRDs into a cluster
.PHONY: install
install: manifests
	kubectl apply -f deploy/crds

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
.PHONY: deploy
deploy:
	kubectl apply -f deploy/ -n $(KIND_NAMESPACE)
	kubectl apply -f deploy/crds/ -n $(KIND_NAMESPACE)
	kubectl set env deployment/$(IMG) -n $(KIND_NAMESPACE) WATCH_NAMESPACE=$(WATCH_NAMESPACE)

.PHONY: create-ns
create-ns:
	@kubectl create namespace $(KIND_NAMESPACE) || true
	@kubectl create namespace $(WATCH_NAMESPACE) || true

############################################################
# unit test
############################################################
GOSEC = $(LOCAL_BIN)/gosec
KUBEBUILDER = $(LOCAL_BIN)/kubebuilder
ENVTEST = $(LOCAL_BIN)/setup-envtest
KBVERSION = 3.12.0
ENVTEST_K8S_VERSION = 1.26.x

.PHONY: test
test: test-dependencies
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test $(TESTARGS) `go list ./... | grep -v test/e2e`

.PHONY: test-coverage
test-coverage: TESTARGS = -json -cover -covermode=atomic -coverprofile=coverage_unit.out
test-coverage: test

.PHONY: test-dependencies
test-dependencies: envtest kubebuilder

.PHONY: kubebuilder
kubebuilder:
	@if [ "$$($(KUBEBUILDER) version 2>/dev/null | grep -o KubeBuilderVersion:\"[0-9]*\.[0-9]\.[0-9]*\")" != "KubeBuilderVersion:\"$(KBVERSION)\"" ]; then \
		echo "Installing Kubebuilder"; \
		curl -L https://github.com/kubernetes-sigs/kubebuilder/releases/download/v$(KBVERSION)/kubebuilder_$(GOOS)_$(GOARCH) -o $(KUBEBUILDER); \
		chmod +x $(KUBEBUILDER); \
	fi

.PHONY: envtest
envtest:
	$(call go-get-tool,sigs.k8s.io/controller-runtime/tools/setup-envtest@latest)

.PHONY: gosec
gosec:
	$(call go-get-tool,github.com/securego/gosec/v2/cmd/gosec@v2.15.0)

.PHONY: gosec-scan
gosec-scan: gosec
	$(GOSEC) -fmt sonarqube -out gosec.json -no-fail -exclude-dir=.go ./...

############################################################
# e2e test
############################################################
GINKGO = $(LOCAL_BIN)/ginkgo

.PHONY: kind-bootstrap-cluster
kind-bootstrap-cluster: kind-create-cluster install-crds kind-deploy-controller install-resources

.PHONY: kind-bootstrap-cluster-dev
kind-bootstrap-cluster-dev: kind-create-cluster install-crds install-resources

.PHONY: kind-deploy-controller
kind-deploy-controller: install-crds
	@echo installing $(IMG)
	kubectl create ns $(KIND_NAMESPACE) || true
	kubectl apply -f deploy/operator.yaml -n $(KIND_NAMESPACE)
	kubectl patch deployment $(IMG) -n $(KIND_NAMESPACE) -p "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"$(IMG)\",\"env\":[{\"name\":\"WATCH_NAMESPACE\",\"value\":\"$(WATCH_NAMESPACE)\"}]}]}}}}"

.PHONY: deploy-controller
deploy-controller: kind-deploy-controller

.PHONY: kind-deploy-controller-dev
kind-deploy-controller-dev: 
	if [ "$(HOSTED)" = "hosted" ]; then\
		$(MAKE) kind-deploy-controller-dev-addon ;\
	else\
		$(MAKE) kind-deploy-controller-dev-normal ;\
	fi

.PHONY: kind-deploy-controller-dev-normal
kind-deploy-controller-dev-normal: kind-deploy-controller
	@echo Pushing image to KinD cluster
	kind load docker-image $(REGISTRY)/$(IMG):$(TAG) --name $(KIND_NAME)
	@echo "Patch deployment image"
	kubectl patch deployment $(IMG) -n $(KIND_NAMESPACE) -p "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"$(IMG)\",\"imagePullPolicy\":\"Never\"}]}}}}"
	kubectl patch deployment $(IMG) -n $(KIND_NAMESPACE) -p "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"$(IMG)\",\"image\":\"$(REGISTRY)/$(IMG):$(TAG)\",\"args\":[]}]}}}}"
	kubectl rollout status -n $(KIND_NAMESPACE) deployment $(IMG) --timeout=180s

.PHONY: kind-deploy-controller-dev-addon
kind-deploy-controller-dev-addon:
	kind load docker-image $(REGISTRY)/$(IMG):$(TAG) --name $(KIND_NAME)
	kubectl annotate -n $(subst -hosted,,$(KIND_NAMESPACE)) --overwrite managedclusteraddon iam-policy-controller\
		addon.open-cluster-management.io/values='{"args": {"frequency": 10}, "global":{"imagePullPolicy": "Never", "imageOverrides":{"iam_policy_controller": "$(REGISTRY)/$(IMG):$(TAG)"}}}'
.PHONY: kind-create-cluster
kind-create-cluster:
	@echo "creating cluster"
	kind create cluster --name $(KIND_NAME) $(KIND_ARGS)
	kind get kubeconfig --name $(KIND_NAME) > $(PWD)/kubeconfig_managed

.PHONY: kind-additional-cluster
kind-additional-cluster:
	@echo "creating cluster"
	kind create cluster --name $(KIND_NAME)2 $(KIND_ARGS)
	kind get kubeconfig --name $(KIND_NAME)2 > $(PWD)/kubeconfig_managed2
	kubectl config use-context kind-$(KIND_NAME)


.PHONY: kind-delete-cluster
kind-delete-cluster:
	kind delete cluster --name $(KIND_NAME)
	kind delete cluster --name $(KIND_NAME)2

.PHONY: install-crds
install-crds:
	@echo installing crds
	kubectl apply -f deploy/crds/policy.open-cluster-management.io_iampolicies.yaml
	kubectl apply -f https://raw.githubusercontent.com/stolostron/governance-policy-propagator/main/deploy/crds/policy.open-cluster-management.io_policies.yaml

.PHONY: install-resources
install-resources:
	@echo creating namespaces
	kubectl create ns $(WATCH_NAMESPACE)

.PHONY: e2e-dependencies
e2e-dependencies:
	$(call go-get-tool,github.com/onsi/ginkgo/v2/ginkgo@$(shell awk '/github.com\/onsi\/ginkgo\/v2/ {print $$2}' go.mod))

.PHONY: e2e-test
e2e-test: e2e-dependencies
	$(GINKGO) -v --fail-fast $(E2E_TEST_ARGS) test/e2e

.PHONY: e2e-test-coverage
e2e-test-coverage: E2E_TEST_ARGS = --json-report=report_e2e.json --label-filter="!hosted-mode" --output-dir=.
e2e-test-coverage: e2e-run-instrumented e2e-test e2e-stop-instrumented

.PHONY: e2e-test-hosted-mode-coverage
e2e-test-hosted-mode-coverage: E2E_TEST_ARGS = --json-report=report_e2e_hosted_mode.json --label-filter="hosted-mode" --output-dir=.
e2e-test-hosted-mode-coverage: COVERAGE_E2E_OUT = coverage_e2e_hosted_mode.out
e2e-test-hosted-mode-coverage: export TARGET_KUBECONFIG_PATH = $(PWD)/kubeconfig_managed2
e2e-test-hosted-mode-coverage: e2e-run-instrumented e2e-test e2e-stop-instrumented

.PHONY: e2e-build-instrumented
e2e-build-instrumented:
	go test -covermode=atomic -coverpkg=$(shell cat go.mod | head -1 | cut -d ' ' -f 2)/... -c -tags e2e ./ -o build/_output/bin/$(IMG)-instrumented

.PHONY: e2e-run-instrumented
e2e-run-instrumented: e2e-build-instrumented
	WATCH_NAMESPACE="$(WATCH_NAMESPACE)" ./build/_output/bin/$(IMG)-instrumented -test.run "^TestRunMain$$" -test.coverprofile=$(COVERAGE_E2E_OUT) &>build/_output/controller.log &

.PHONY: e2e-stop-instrumented
e2e-stop-instrumented:
	ps -ef | grep '$(IMG)' | grep -v grep | awk '{print $$2}' | xargs kill

.PHONY: e2e-debug
e2e-debug:
	kubectl get all -n $(KIND_NAMESPACE)
	kubectl get leases -n $(KIND_NAMESPACE)
	kubectl get all -n $(WATCH_NAMESPACE)
	kubectl get iampolicies.policy.open-cluster-management.io --all-namespaces
	kubectl describe pods -n $(KIND_NAMESPACE)
	kubectl logs $$(kubectl get pods -n $(KIND_NAMESPACE) -o name | grep $(IMG)) -n $(KIND_NAMESPACE)

############################################################
# test coverage
############################################################
GOCOVMERGE = $(LOCAL_BIN)/gocovmerge
.PHONY: coverage-dependencies
coverage-dependencies:
	$(call go-get-tool,github.com/wadey/gocovmerge@v0.0.0-20160331181800-b5bfa59ec0ad)

COVERAGE_FILE = coverage.out
.PHONY: coverage-merge
coverage-merge: coverage-dependencies
	@echo Merging the coverage reports into $(COVERAGE_FILE)
	$(GOCOVMERGE) $(PWD)/coverage_* > $(COVERAGE_FILE)

.PHONY: coverage-verify
coverage-verify:
	./build/common/scripts/coverage_calc.sh

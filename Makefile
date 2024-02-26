# IBM Confidential
# OCO Source Materials
# (C) Copyright IBM Corporation 2018 All Rights Reserved
# The source code for this program is not published or otherwise divested of its trade secrets, irrespective of what has been deposited with the U.S. Copyright Office.
# Copyright (c) 2020 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

PWD := $(shell pwd)
LOCAL_BIN ?= $(PWD)/bin

export PATH := $(LOCAL_BIN):$(PATH)
GOARCH = $(shell go env GOARCH)
GOOS = $(shell go env GOOS)

TESTARGS_DEFAULT := -v
export TESTARGS ?= $(TESTARGS_DEFAULT)

# Handle KinD configuration
CONTROLLER_NAME ?= $(shell cat COMPONENT_NAME 2> /dev/null)
CONTROLLER_NAMESPACE ?= open-cluster-management-agent-addon
KIND_NAMESPACE ?= $(CONTROLLER_NAMESPACE)
MANAGED_CLUSTER_SUFFIX ?= 
MANAGED_CLUSTER_NAME ?= managed$(MANAGED_CLUSTER_SUFFIX)
WATCH_NAMESPACE ?= $(MANAGED_CLUSTER_NAME)

# Test coverage threshold
export COVERAGE_MIN ?= 53
COVERAGE_E2E_OUT ?= coverage_e2e.out

# Image URL to use all building/pushing image targets;
# Use your own docker registry and image name for dev/test by overridding the IMG and REGISTRY environment variable.
IMG ?= $(CONTROLLER_NAME)
REGISTRY ?= quay.io/stolostron
TAG ?= latest
IMAGE_NAME_AND_VERSION ?= $(REGISTRY)/$(IMG)

include build/common/Makefile.common.mk

.PHONY: all
all: test

.PHONY: clean
clean:
	-rm bin/*
	-rm build/_output/bin/*
	-rm coverage*.out
	-rm report*.json
	-rm kubeconfig_*
	-rm -r vendor/

############################################################
# lint
############################################################

.PHONY: lint
lint:

.PHONY: fmt
fmt:

.PHONY: vet
vet:
	go vet ./...

############################################################
# build
############################################################

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

.PHONY: manifests
manifests: controller-gen kustomize
	$(CONTROLLER_GEN) crd rbac:roleName=iam-policy-controller paths="./..." output:crd:artifacts:config=deploy/crds/kustomize output:rbac:artifacts:config=deploy/rbac
	$(KUSTOMIZE) build deploy/crds/kustomize > deploy/crds/policy.open-cluster-management.io_iampolicies.yaml

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: generate-operator-yaml
generate-operator-yaml: kustomize manifests
	$(KUSTOMIZE) build deploy/manager > deploy/operator.yaml

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
	# Creating namespaces
	-kubectl create namespace $(KIND_NAMESPACE)
	-kubectl create namespace $(WATCH_NAMESPACE)

############################################################
# unit test
############################################################

.PHONY: test
test: test-dependencies
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test $(TESTARGS) `go list ./... | grep -v test/e2e`

.PHONY: test-coverage
test-coverage: TESTARGS = -json -cover -covermode=atomic -coverprofile=coverage_unit.out
test-coverage: test

.PHONY: test-dependencies
test-dependencies: envtest kubebuilder

.PHONY: gosec-scan
gosec-scan:

############################################################
# e2e test
############################################################
GINKGO = $(LOCAL_BIN)/ginkgo

.PHONY: kind-bootstrap-cluster
kind-bootstrap-cluster: kind-bootstrap-cluster-dev kind-deploy-controller

.PHONY: kind-bootstrap-cluster-dev
kind-bootstrap-cluster-dev: CLUSTER_NAME = $(MANAGED_CLUSTER_NAME)
kind-bootstrap-cluster-dev: kind-create-cluster install-crds kind-controller-kubeconfig

.PHONY: kind-deploy-controller
kind-deploy-controller: deploy

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

.PHONY: kind-additional-cluster
kind-additional-cluster: MANAGED_CLUSTER_SUFFIX = 2
kind-additional-cluster: CLUSTER_NAME = $(MANAGED_CLUSTER_NAME)
kind-additional-cluster: kind-create-cluster kind-controller-kubeconfig

.PHONY: kind-delete-cluster
kind-delete-cluster: CLUSTER_NAME = $(MANAGED_CLUSTER_NAME)
kind-delete-cluster:
	-kind delete cluster --name $(KIND_NAME)
	-kind delete cluster --name $(KIND_NAME)2

.PHONY: install-crds
install-crds:
	@echo installing crds
	kubectl apply -f deploy/crds/policy.open-cluster-management.io_iampolicies.yaml
	kubectl apply -f https://raw.githubusercontent.com/stolostron/governance-policy-propagator/main/deploy/crds/policy.open-cluster-management.io_policies.yaml

.PHONY: install-resources
install-resources: create-ns
	# deploying roles and service account
	kubectl apply -k deploy/rbac
	kubectl apply -f deploy/manager/service_account.yaml -n $(KIND_NAMESPACE)

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

COVERAGE_FILE = coverage.out
.PHONY: coverage-merge
coverage-merge: coverage-dependencies
	@echo Merging the coverage reports into $(COVERAGE_FILE)
	$(GOCOVMERGE) $(PWD)/coverage_* > $(COVERAGE_FILE)

.PHONY: coverage-verify
coverage-verify:
	./build/common/scripts/coverage_calc.sh

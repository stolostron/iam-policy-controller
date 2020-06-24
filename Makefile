# IBM Confidential
# OCO Source Materials
# (C) Copyright IBM Corporation 2018 All Rights Reserved
# The source code for this program is not published or otherwise divested of its trade secrets, irrespective of what has been deposited with the U.S. Copyright Office.
# Copyright (c) 2020 Red Hat, Inc.

# Git vars
GITHUB_USER ?=
GITHUB_TOKEN ?=

# CICD BUILD HARNESS
####################
GITHUB_USER := $(shell echo $(GITHUB_USER) | sed 's/@/%40/g')

USE_VENDORIZED_BUILD_HARNESS ?=

ifndef USE_VENDORIZED_BUILD_HARNESS
-include $(shell curl -s -H 'Authorization: token ${GITHUB_TOKEN}' -H 'Accept: application/vnd.github.v4.raw' -L https://api.github.com/repos/open-cluster-management/build-harness-extensions/contents/templates/Makefile.build-harness-bootstrap -o .build-harness-bootstrap; echo .build-harness-bootstrap)
else
-include vbh/.build-harness-vendorized
endif

.PHONY: default
default::
	@echo "Build Harness Bootstrapped"


# Go build settings
ARCH = $(shell uname -m)
ifeq ($(ARCH), x86_64)
    ARCH = amd64
endif

GOARCH = $(shell go env GOARCH)
GOOS = $(shell go env GOOS)

# Only use git commands if it exists
ifdef GIT
GIT_COMMIT      = $(shell git rev-parse --short HEAD)
GIT_REMOTE_URL  = $(shell git config --get remote.origin.url)
endif

.PHONY: all lint test dependencies build image run deploy install fmt vet generate

all: test

lint:
	@echo "Linting disabled."

copyright-check:
	./build/copyright-check.sh $(TRAVIS_BRANCH) $(TRAVIS_PULL_REQUEST_BRANCH)

# Run tests
test:  dependencies
	go test -v  -coverprofile=coverage.out  ./...

dependencies:
	curl -sL https://go.kubebuilder.io/dl/2.0.0-alpha.1/${GOOS}/${GOARCH} | tar -xz -C /tmp/
	sudo mv /tmp/kubebuilder_2.0.0-alpha.1_${GOOS}_${GOARCH} /usr/local/kubebuilder
	go mod tidy
	go mod download

build:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -a -tags netgo -o ./iam-policy ./cmd/manager

local-test-image: export COMPONENT_INIT_COMMAND=./build/install-dependencies.sh
local-test-image: export COMPONENT_BUILD_COMMAND=./build/build.sh
local-test-image: export COMPONENT_TAG_EXTENSION=-localtest
local-test-image: export GOOS=linux
local-test-image:
	@make component/build

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet
	go run ./cmd/manager/main.go

# Install CRDs into a cluster
install: manifests
	kubectl apply -f config/crds

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	kubectl apply -f config/crds
	kustomize build config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests:
	go run vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go all

# Run go fmt against code
fmt:
	go fmt ./pkg/... ./cmd/...

# Run go vet against code
vet:
	go vet ./pkg/... ./cmd/...

# Generate code
generate:
	go generate ./pkg/... ./cmd/...

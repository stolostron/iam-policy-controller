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
GOARCH := $(ARCH)
GOOS := linux

# Only use git commands if it exists
ifdef GIT
GIT_COMMIT      = $(shell git rev-parse --short HEAD)
GIT_REMOTE_URL  = $(shell git config --get remote.origin.url)
endif

# Image settings

IMAGE_NAME ?= iam-policy-controller
IMAGE_VERSION := 1.0.0
IMAGE_DESCRIPTION =IAM Policy Controller

DOCKER_REGISTRY ?= quay.io
DOCKER_NAMESPACE ?= open-cluster-management
DOCKER_BUILD_TAG ?= $(IMAGE_VERSION)-$(GIT_COMMIT)
DOCKER_FILE := Dockerfile
DOCKER_BUILD_DIR := $(TRAVIS_BUILD_DIR)
DOCKER_BUILD_OPTS=--build-arg "VCS_REF=$(GIT_COMMIT)" \
	--build-arg "VCS_URL=$(GIT_REMOTE_URL)" \
	--build-arg "IMAGE_NAME=$(IMAGE_NAME)" \
	--build-arg "IMAGE_DESCRIPTION=$(IMAGE_DESCRIPTION)" \
	--build-arg "SUMMARY=$(IMAGE_DESCRIPTION)" \
	--build-arg "GOARCH=$(GOARCH)"


.PHONY: all lint test dependencies build image run deploy install fmt vet generate

all: test

lint:
	@echo "Linting disabled."

copyright-check:
	./build/copyright-check.sh $(TRAVIS_BRANCH) $(TRAVIS_PULL_REQUEST_BRANCH)

install-testdependencies:
	curl -sL https://go.kubebuilder.io/dl/2.0.0-alpha.1/${GOOS}/${GOARCH} | tar -xz -C /tmp/
	sudo mv /tmp/kubebuilder_2.0.0-alpha.1_${GOOS}_${GOARCH} /usr/local/kubebuilder

# Run tests
test:  fmt vet
	go test ./pkg/... ./cmd/... -v -coverprofile cover.out

dependencies:
	go mod tidy
	go mod download

build:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -a -tags netgo -o ./iam-policy_$(GOARCH) ./cmd/manager

image:
	@make DOCKER_BUILD_OPTS=$(DOCKER_BUILD_OPTS) docker:build
	@make docker:tag

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

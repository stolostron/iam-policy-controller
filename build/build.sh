#!/bin/bash
set -e

export DOCKER_IMAGE_AND_TAG=${1}
#already packaged dependencies, dont think this should run during build
#make dependencies
make build
make docker/build

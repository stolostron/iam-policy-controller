#!/bin/bash
set -e

make dependencies
make lint
make test

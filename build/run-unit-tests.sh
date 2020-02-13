#!/bin/bash
set -e

make deps
make lintall
make test
make coverage
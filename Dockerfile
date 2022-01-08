# Copyright Contributors to the Open Cluster Management project

# Stage 1: Use image builder to build the target binaries
FROM golang:1.17 AS builder

ENV COMPONENT=iam-policy-controller
ENV REPO_PATH=/go/src/github.com/stolostron/${COMPONENT}
WORKDIR ${REPO_PATH}
COPY . .
RUN make build

# Stage 2: Copy the binaries from the image builder to the base image
FROM registry.access.redhat.com/ubi8/ubi-minimal:latest

ENV COMPONENT=iam-policy-controller
ENV REPO_PATH=/go/src/github.com/stolostron/${COMPONENT}
ENV OPERATOR=/usr/local/bin/${COMPONENT} \
    USER_UID=1001 \
    USER_NAME=${COMPONENT}

# install operator binary
COPY --from=builder ${REPO_PATH}/build/_output/bin/${COMPONENT} ${OPERATOR}

COPY --from=builder ${REPO_PATH}/build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup

RUN microdnf update && \
    microdnf install shadow-utils procps && \
    microdnf clean all

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}

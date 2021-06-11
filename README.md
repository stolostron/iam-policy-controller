[comment]: # ( Copyright Contributors to the Open Cluster Management project )

# IAM Policy Controller

[![Build](https://img.shields.io/badge/build-Prow-informational)](https://prow.ci.openshift.org/?repo=open-cluster-management%2Fcert-policy-controller) [![KinD tests](https://github.com/open-cluster-management/iam-policy-controller/actions/workflows/kind.yml/badge.svg?branch=main&event=push)](https://github.com/open-cluster-management/iam-policy-controller/actions/workflows/kind.yml) [![License](https://img.shields.io/:license-apache-blue.svg)](http://www.apache.org/licenses/LICENSE-2.0.html)

## Description

The IAM policy controller is a controller that watches `IamPolicies` created to monitor IAM cluster role bindings used within a Kubernetes cluster. It detects the number of users with cluster role bindings to a given clusterrole and reports whether or not a given `IamPolicy` is compliant or noncompliant.The controller can be run as a stand-alone program or as an integrated part of governing risk with the Open Cluster Management project.

| Field | Description |
| ---- | ---- |
| maxClusterRoleBindingUsers | Required: Maximum number of cluster role binding still valid before it is considered as non-compliant. |
| ClusterRole | Optional: Cluster role referenced in the cluster role bindings, default to cluster-admin. |

Following is an example spec of a `IamPolicy` resource:

```yaml
apiVersion: policy.open-cluster-management.io/v1
kind: IamPolicy
metadata:
  name: iam-grc-policy
  label:
    category: "System-Integrity"
spec:
  # Include are the namespaces for which you want to watch cluster administrator role and IAM rolebinings, while exclude are the namespaces you explicitly do not want to watch
  namespaceSelector:
    include: ["default","kube-*"]
    exclude: ["kube-system"]
  #labelSelector:
    #env: "production"
  # Can be enforce or inform, however enforce doesn't do anything with regards to this controller
     remediationAction: inform # enforce or inform
     severity: medium # low, medium, or high
  # Maximum number of cluster role binding still valid before it is considered as non-compliant
  maxClusterRoleBindingUsers: 5
```
Go to the [Contributing guide](CONTRIBUTING.md) to learn how to get involved.

## Getting started

### Steps for development

  - Build code
    ```bash
    make build
    ```
  - Run controller locally against the Kubernetes cluster currently configured with `kubectl`
    ```bash
    export WATCH_NAMESPACE=<namespace>
    make run
    ```
    (`WATCH_NAMESPACE` can be any namespace on the cluster that you want the controller to monitor for policies.)


### Steps for deployment

  - Build container image
    ```bash
    make build-images
    ```
    - The image registry, name, and tag used in the image build, are configurable with:
      ```bash
      export REGISTRY=''  # (defaults to 'quay.io/open-cluster-management')
      export IMG=''       # (defaults to the repository name)
      export TAG=''       # (defaults to 'latest')
      ```
  - Deploy controller to a cluster

    The controller is deployed to a namespace defined in `CONTROLLER_NAMESPACE` and monitors the namepace defined in `WATCH_NAMESPACE` for `IamPolicy` resources.

    1. Create the deployment namespaces
       ```bash
       make create-ns
       ```
       The deployment namespaces are configurable with:
       ```bash
       export CONTROLLER_NAMESPACE=''  # (defaults to 'open-cluster-management-agent-addon')
       export WATCH_NAMESPACE=''       # (defaults to 'managed')
       ```
    2. Deploy the controller and related resources
       ```bash
       make deploy
       ```
    **NOTE:** Please be aware of the community's [deployment images](https://github.com/open-cluster-management/community#deployment-images) special note.


### Steps for test

  - Code linting
    ```bash
    make lint
    ```
  - Unit tests
    - Install prerequisites
      ```bash
      make test-dependencies
      ```
    - Run unit tests
      ```bash
      make test
      ```
  - E2E tests (**NOTE:** Currently there are no E2E tests to run)
    1. Prerequisites:
       - [docker](https://docs.docker.com/get-docker/)
       - [kind](https://kind.sigs.k8s.io/docs/user/quick-start/)
    2. Start KinD cluster (make sure Docker is running first)
       ```bash
       make kind-bootstrap-cluster-dev
       ```
    3. Start the controller locally (see [Steps for development](#steps-for-development))
    4. Run E2E tests:
       ```bash
       export WATCH_NAMESPACE=managed
       make e2e-test
       ```

## References

- The `iam-policy-controller` is part of the `open-cluster-management` community. For more information, visit: [open-cluster-management.io](https://open-cluster-management.io).
- Check the [Security guide](SECURITY.md) if you need to report a security issue.




<!---
Date: June/11/2021
-->

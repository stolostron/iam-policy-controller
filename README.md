# IAM Policy Controller

## Description
IAM policy controller watches cluster administrator role and IAM role binding created and used within ICP/MCM. It detects the cluster administrator role and IAM role binding violations and report it. The controller shows whether or not a given `IamPolicy` is compliant or noncompliant.

## Usage
The controller can be run as a stand-alone program within IBM Cloud Private. Its intended usage is to be integrated with Multi-cloud Manager.

`IamPolicy` is kind for the custom resource definition created by this controller. It watches the namespaces included in namespace selector and shows whether those namespaces and the policy as a whole is compliant or not.

The controller watches for policy registered with kind `IamPolicy` objects in Kubernetes. Following is an example spec of a `IamPolicy` object:

```yaml
apiVersion: iam.mcm.ibm.com/v1alpha1
kind: IamPolicy
metadata:
  name: iam-grc-policy
  namespace: kube-system
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
  # Maximum number of cluster role binding still valid before it is considered as non-compliant
  maxClusterRoleBindingUsers: 5
  # Maximum number of iam role bindings still valid before it is considered as non-compliant
  maxRoleBindingViolationPerNamespace: 2
```

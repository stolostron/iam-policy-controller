module github.com/open-cluster-management/iam-policy-controller

go 1.16

require (
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.13.0
	github.com/open-cluster-management/addon-framework v0.0.0-20210621074027-a81f712c10c2
	github.com/open-cluster-management/governance-policy-propagator v0.0.0-20211012174109-95c3b77cce09
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	k8s.io/api v0.21.3
	k8s.io/apimachinery v0.21.3
	k8s.io/client-go v12.0.0+incompatible
	sigs.k8s.io/controller-runtime v0.9.2
)

replace (
	github.com/go-logr/zapr => github.com/go-logr/zapr v0.4.0
	k8s.io/client-go => k8s.io/client-go v0.21.3
)

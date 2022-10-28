// Copyright Contributors to the Open Cluster Management project

package e2e

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbac "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var _ = Describe("Test an alternative kubeconfig for policy evaluation", Ordered, Label("hosted-mode"), func() {
	const (
		envName    = "TARGET_KUBECONFIG_PATH"
		policyName = "test-hosted-mode"
		policyYAML = "../resources/case1_alternative_kubeconfig/iam-policy-hosted.yaml"
	)

	var targetK8sClient *kubernetes.Clientset

	BeforeAll(func() {
		By("Checking that the " + envName + " environment variable is valid")
		altKubeconfigPath := os.Getenv(envName)
		Expect(altKubeconfigPath).ToNot(Equal(""))

		targetK8sConfig, err := clientcmd.BuildConfigFromFlags("", altKubeconfigPath)
		Expect(err).To(BeNil())

		targetK8sClient, err = kubernetes.NewForConfig(targetK8sConfig)
		Expect(err).To(BeNil())
	})

	AfterAll(func() {
		err := targetK8sClient.RbacV1().ClusterRoles().Delete(
			context.TODO(), "hostedrole", metav1.DeleteOptions{},
		)
		if !errors.IsNotFound(err) {
			Expect(err).To(BeNil())
		}
		err = targetK8sClient.RbacV1().ClusterRoleBindings().Delete(
			context.TODO(), "hostedbinding", metav1.DeleteOptions{},
		)
		if !errors.IsNotFound(err) {
			Expect(err).To(BeNil())
		}
	})

	It("should verify compliance on alternative kubeconfig", func() {
		By("Creating the Policy on hosting cluster")
		Kubectl("apply", "-f", policyYAML, "-n", testNamespace)
		pol := GetWithTimeout(gvrIamPolicy, policyName, testNamespace, true)
		Expect(pol).NotTo(BeNil())

		By("Verifying  the Policy is compliant initially")
		Eventually(func() interface{} {
			pol := GetWithTimeout(gvrIamPolicy, policyName, testNamespace, true)
			comp, _, _ := unstructured.NestedString(pol.Object, "status", "compliant")

			return comp
		}, defaultTimeoutSeconds, 1).Should(Equal("Compliant"))

		By("Creating the clusterrolebindings on the hosted cluster")
		cr := rbac.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: "hostedcr",
			},
			Rules: []rbac.PolicyRule{
				{
					Verbs:           []string{"get"},
					NonResourceURLs: []string{"/testhosted"},
				},
			},
		}
		_, err := targetK8sClient.RbacV1().ClusterRoles().Create(context.TODO(), &cr, metav1.CreateOptions{})
		Expect(err).To(BeNil())

		crb := rbac.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: "hostedcrb",
			},
			Subjects: []rbac.Subject{
				{APIGroup: "rbac.authorization.k8s.io", Kind: "User", Name: "user1"},
				{APIGroup: "rbac.authorization.k8s.io", Kind: "User", Name: "user2"},
			},
			RoleRef: rbac.RoleRef{
				Kind:     "ClusterRole",
				Name:     "hostedcr",
				APIGroup: "rbac.authorization.k8s.io",
			},
		}

		_, err = targetK8sClient.RbacV1().ClusterRoleBindings().Create(context.TODO(), &crb, metav1.CreateOptions{})
		Expect(err).To(BeNil())

		By("Verifying that Policy is now Non-compliant")
		Eventually(func() interface{} {
			pol := GetWithTimeout(gvrIamPolicy, policyName, testNamespace, true)
			comp, _, _ := unstructured.NestedString(pol.Object, "status", "compliant")

			return comp
		}, defaultTimeoutSeconds, 1).Should(Equal("NonCompliant"))
	})
})

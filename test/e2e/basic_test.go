// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package e2e

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var _ = Describe("Test IAM policy", func() {
	complianceTests := func(policyName, additionalBindingsYaml string) {
		It("Should become compliant at first", func() {
			Eventually(func() interface{} {
				pol := GetWithTimeout(gvrIamPolicy, policyName, testNamespace, true)
				comp, _, _ := unstructured.NestedString(pol.Object, "status", "compliant")

				return comp
			}, defaultTimeoutSeconds, 1).Should(Equal("Compliant"))
		})
		It("Should become noncompliant after creating more cluster-admin bindings", func() {
			Kubectl("apply", "-f", additionalBindingsYaml)
			Eventually(func() interface{} {
				pol := GetWithTimeout(gvrIamPolicy, policyName, testNamespace, true)
				comp, _, _ := unstructured.NestedString(pol.Object, "status", "compliant")

				return comp
			}, defaultTimeoutSeconds, 1).Should(Equal("NonCompliant"))
		})
		It("Should become compliant after the bindings are removed", func() {
			Kubectl("delete", "-f", additionalBindingsYaml)
			Eventually(func() interface{} {
				pol := GetWithTimeout(gvrIamPolicy, policyName, testNamespace, true)
				comp, _, _ := unstructured.NestedString(pol.Object, "status", "compliant")

				return comp
			}, defaultTimeoutSeconds, 1).Should(Equal("Compliant"))
		})
	}

	Describe("Test default clusterrole (cluster-admin)", Ordered, func() {
		const (
			adminIAMPolicyYaml string = "../resources/iam-policy-default.yaml"
			adminIAMPolicyName string = "rule-of-two"
			adminCRBYaml       string = "../resources/admin-crb.yaml"
		)

		It("Should be created on the cluster", func() {
			Kubectl("apply", "-f", adminIAMPolicyYaml, "-n", testNamespace)
			pol := GetWithTimeout(gvrIamPolicy, adminIAMPolicyName, testNamespace, true)
			Expect(pol).NotTo(BeNil())
		})
		complianceTests(adminIAMPolicyName, adminCRBYaml)
		AfterAll(func() {
			Kubectl("delete", "-f", adminCRBYaml, "--ignore-not-found", "-n", testNamespace)
			Kubectl("delete", "-f", adminIAMPolicyYaml, "--ignore-not-found", "-n", testNamespace)
		})
	})
	Describe("Test specified clusterrole", Ordered, func() {
		const (
			otherIAMPolicyYaml string = "../resources/iam-policy-other.yaml"
			otherIAMPolicyName string = "there-can-be-only-one"
			otherCRBYaml       string = "../resources/other-crb.yaml"
			otherRoleYaml      string = "../resources/other-clusterrole.yaml"
		)

		It("Should be created on the cluster", func() {
			Kubectl("apply", "-f", otherIAMPolicyYaml, "-n", testNamespace)
			Kubectl("apply", "-f", otherRoleYaml, "-n", testNamespace)
			pol := GetWithTimeout(gvrIamPolicy, otherIAMPolicyName, testNamespace, true)
			Expect(pol).NotTo(BeNil())
		})
		complianceTests(otherIAMPolicyName, otherCRBYaml)
		AfterAll(func() {
			Kubectl("delete", "-f", otherCRBYaml, "--ignore-not-found", "-n", testNamespace)
			Kubectl("delete", "-f", otherIAMPolicyYaml, "--ignore-not-found", "-n", testNamespace)
			Kubectl("delete", "-f", otherRoleYaml, "--ignore-not-found", "-n", testNamespace)
		})
	})
})

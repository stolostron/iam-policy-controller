// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package e2e

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var _ = Describe("Test IAM policy", func() {
	complianceTests := func(policyName, policyParentName, additionalBindingsYaml, expectedMsg string) {
		It("Should become compliant at first", func() {
			By("Checking policy status")
			Eventually(func() interface{} {
				pol := GetWithTimeout(gvrIamPolicy, policyName, testNamespace, true)
				comp, _, _ := unstructured.NestedString(pol.Object, "status", "compliant")

				return comp
			}, defaultTimeoutSeconds, 1).Should(Equal("Compliant"))

			By("Checking parent policy events")
			Eventually(func() interface{} {
				return GetMatchingEvents(testNamespace, policyParentName,
					"policy: "+testNamespace+"/"+policyName, "^Compliant;")
			}, defaultTimeoutSeconds, 1).ShouldNot(BeEmpty())
			Consistently(func() interface{} {
				return GetMatchingEvents(testNamespace, policyParentName,
					"policy: "+testNamespace+"/"+policyName, "^NonCompliant;")
			}, "15s", 1).Should(BeEmpty())
		})
		It("Should become noncompliant after creating more bindings", func() {
			Kubectl("apply", "-f", additionalBindingsYaml)

			By("Checking policy status")
			Eventually(func() interface{} {
				pol := GetWithTimeout(gvrIamPolicy, policyName, testNamespace, true)
				comp, _, _ := unstructured.NestedString(pol.Object, "status", "compliant")

				return comp
			}, defaultTimeoutSeconds, 1).Should(Equal("NonCompliant"))

			By("Checking parent policy events")
			Eventually(func() interface{} {
				return GetMatchingEvents(testNamespace, policyParentName,
					"policy: "+testNamespace+"/"+policyName, "^NonCompliant;.*"+expectedMsg)
			}, defaultTimeoutSeconds, 1).ShouldNot(BeEmpty())
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
			adminPolicyYaml    string = "../resources/parent-policy-default.yaml"
			adminPolicyName    string = "rule-of-two-parent"
			adminIAMPolicyYaml string = "../resources/iam-policy-default.yaml"
			adminIAMPolicyName string = "rule-of-two"
			adminCRBYaml       string = "../resources/admin-crb.yaml"
		)

		It("Should be created on the cluster", func() {
			CreateIAMPolicyWithParent(adminPolicyYaml, adminPolicyName, adminIAMPolicyYaml)
		})
		complianceTests(adminIAMPolicyName, adminPolicyName, adminCRBYaml, "at least 1 above")
		AfterAll(func() {
			Kubectl("delete", "-f", adminCRBYaml, "--ignore-not-found", "-n", testNamespace)
			Kubectl("delete", "-f", adminIAMPolicyYaml, "--ignore-not-found", "-n", testNamespace)
			Kubectl("delete", "-f", adminPolicyYaml, "--ignore-not-found", "-n", testNamespace)
			Kubectl("delete", "event", "--field-selector=involvedObject.name="+adminPolicyName, "-n", testNamespace)
			Kubectl("delete", "event", "--field-selector=involvedObject.name="+adminIAMPolicyName, "-n", testNamespace)
		})
	})
	Describe("Test specified clusterrole", Ordered, func() {
		const (
			otherPolicyYaml    string = "../resources/parent-policy-other.yaml"
			otherPolicyName    string = "parent-policy-other"
			otherIAMPolicyYaml string = "../resources/iam-policy-other.yaml"
			otherIAMPolicyName string = "there-can-be-only-one"
			otherCRBYaml       string = "../resources/other-crb.yaml"
			otherRoleYaml      string = "../resources/other-clusterrole.yaml"
		)

		It("Should be created on the cluster", func() {
			CreateIAMPolicyWithParent(otherPolicyYaml, otherPolicyName, otherIAMPolicyYaml)
			Kubectl("apply", "-f", otherRoleYaml, "-n", testNamespace)
		})
		complianceTests(otherIAMPolicyName, otherPolicyName, otherCRBYaml, "at least 10 above")
		AfterAll(func() {
			Kubectl("delete", "-f", otherCRBYaml, "--ignore-not-found", "-n", testNamespace)
			Kubectl("delete", "-f", otherIAMPolicyYaml, "--ignore-not-found", "-n", testNamespace)
			Kubectl("delete", "-f", otherRoleYaml, "--ignore-not-found", "-n", testNamespace)
			Kubectl("delete", "-f", otherPolicyYaml, "--ignore-not-found", "-n", testNamespace)
			Kubectl("delete", "event", "--field-selector=involvedObject.name="+otherPolicyName, "-n", testNamespace)
			Kubectl("delete", "event", "--field-selector=involvedObject.name="+otherIAMPolicyName, "-n", testNamespace)
		})
	})
})

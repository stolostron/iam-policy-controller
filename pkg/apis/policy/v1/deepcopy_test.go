// Licensed Materials - Property of IBM
// (c) Copyright IBM Corporation 2018. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package v1

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var iamPolicy = IamPolicy{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "foo",
		Namespace: "default",
	}}

var iamPolicySpec = IamPolicySpec{
	Severity:                        "high",
	RemediationAction:               "enforce",
	MaxClusterRoleBindingUsers:     1,
}

var typeMeta = metav1.TypeMeta{
	Kind:       "Policy",
	APIVersion: "v1",
}

var objectMeta = metav1.ObjectMeta{
	Name:      "foo",
	Namespace: "default",
}

var listMeta = metav1.ListMeta{
	Continue: "continue",
}

var items = []IamPolicy{}

func TestPolicyDeepCopyInto(t *testing.T) {
	policy := Policy{
		ObjectMeta: objectMeta,
		TypeMeta:   typeMeta,
	}
	policy2 := Policy{}
	policy.DeepCopyInto(&policy2)
	assert.True(t, reflect.DeepEqual(policy, policy2))
}

func TestPolicyDeepCopy(t *testing.T) {
	typeMeta := metav1.TypeMeta{
		Kind:       "Policy",
		APIVersion: "v1",
	}

	objectMeta := metav1.ObjectMeta{
		Name:      "foo",
		Namespace: "default",
	}

	policy := Policy{
		ObjectMeta: objectMeta,
		TypeMeta:   typeMeta,
	}
	policy2 := policy.DeepCopy()
	assert.True(t, reflect.DeepEqual(policy, *policy2))
}

func TestIamPolicyDeepCopyInto(t *testing.T) {
	policy2 := IamPolicy{}
	iamPolicy.DeepCopyInto(&policy2)
	assert.True(t, reflect.DeepEqual(iamPolicy, policy2))
}

func TestIamPolicyDeepCopy(t *testing.T) {
	policy := IamPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		}}
	policy2 := policy.DeepCopy()
	assert.True(t, reflect.DeepEqual(policy, *policy2))
}

func TestIamPolicySpecDeepCopyInto(t *testing.T) {
	policySpec2 := IamPolicySpec{}
	iamPolicySpec.DeepCopyInto(&policySpec2)
	assert.True(t, reflect.DeepEqual(iamPolicySpec, policySpec2))
}

func TestIamPolicySpecDeepCopy(t *testing.T) {
	policySpec2 := iamPolicySpec.DeepCopy()
	assert.True(t, reflect.DeepEqual(iamPolicySpec, *policySpec2))
}

func TestIamPolicyListDeepCopy(t *testing.T) {
	items = append(items, iamPolicy)
	iamPolicyList := IamPolicyList{
		TypeMeta: typeMeta,
		ListMeta: listMeta,
		Items:    items,
	}
	iamPolicyList2 := iamPolicyList.DeepCopy()
	assert.True(t, reflect.DeepEqual(iamPolicyList, *iamPolicyList2))
}

func TestIamPolicyListDeepCopyInto(t *testing.T) {
	items = append(items, iamPolicy)
	iamPolicyList := IamPolicyList{
		TypeMeta: typeMeta,
		ListMeta: listMeta,
		Items:    items,
	}
	iamPolicyList2 := IamPolicyList{}
	iamPolicyList.DeepCopyInto(&iamPolicyList2)
	assert.True(t, reflect.DeepEqual(iamPolicyList, iamPolicyList2))
}

func TestIamPolicyStatusDeepCopy(t *testing.T) {
	var compliantDetail = map[string][]string{}
	var compliantDetails = map[string]map[string][]string{}
	details := []string{}

	details = append(details, "detail1", "detail2")

	compliantDetail["w"] = details
	compliantDetails["a"] = compliantDetail
	compliantDetails["b"] = compliantDetail
	compliantDetails["c"] = compliantDetail
	iamPolicyStatus := IamPolicyStatus{
		ComplianceState:   "Compliant",
		CompliancyDetails: compliantDetails,
	}
	iamPolicyStatus2 := iamPolicyStatus.DeepCopy()
	assert.True(t, reflect.DeepEqual(iamPolicyStatus, *iamPolicyStatus2))
}

func TestIamPolicyStatusDeepCopyInto(t *testing.T) {
	var compliantDetail = map[string][]string{}
	var compliantDetails = map[string]map[string][]string{}
	details := []string{}

	details = append(details, "detail1", "detail2")

	compliantDetail["w"] = details
	compliantDetails["a"] = compliantDetail
	compliantDetails["b"] = compliantDetail
	compliantDetails["c"] = compliantDetail
	iamPolicyStatus := IamPolicyStatus{
		ComplianceState:   "Compliant",
		CompliancyDetails: compliantDetails,
	}
	var iamPolicyStatus2 IamPolicyStatus
	iamPolicyStatus.DeepCopyInto(&iamPolicyStatus2)
	assert.True(t, reflect.DeepEqual(iamPolicyStatus, iamPolicyStatus2))
}

// Licensed Materials - Property of IBM
// (c) Copyright IBM Corporation 2018. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"testing"

	iampolicyv1 "github.com/open-cluster-management/iam-policy-controller/api/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestConvertPolicyStatusToString(t *testing.T) {
	instance := &iampolicyv1.IamPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: iampolicyv1.IamPolicySpec{
			MaxClusterRoleBindingUsers: 1,
		},
	}
	var compliantDetail = map[string][]string{}
	var compliantDetails = map[string]iampolicyv1.CompliancyDetail{}
	details := []string{}

	details = append(details, "detail1", "detail2")

	compliantDetail["w"] = details
	compliantDetails["a"] = compliantDetail
	compliantDetails["b"] = compliantDetail
	compliantDetails["c"] = compliantDetail
	iamPolicyStatus := iampolicyv1.IamPolicyStatus{
		ComplianceState:   "Compliant",
		CompliancyDetails: compliantDetails,
	}
	instance.Status = iamPolicyStatus
	var policyInString = convertPolicyStatusToString(instance)
	assert.NotNil(t, policyInString)

	instance.Status.ComplianceState = ""
	policyInString = convertPolicyStatusToString(instance)
	assert.True(t, policyInString == "ComplianceState is still undetermined")

	instance.Status.CompliancyDetails = nil
	policyInString = convertPolicyStatusToString(instance)
	assert.True(t, policyInString == "ComplianceState is still undetermined")

	instance.Status = iamPolicyStatus
	instance.Status.ComplianceState = "NonCompliant"
	assert.NotNil(t, policyInString)

}

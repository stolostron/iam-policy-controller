// Licensed Materials - Property of IBM
// (c) Copyright IBM Corporation 2018. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"fmt"
	"strings"

	iampolicyv1 "github.com/open-cluster-management/iam-policy-controller/api/v1"
)

//=================================================================
// convertPolicyStatusToString to be able to pass the status as event
func convertPolicyStatusToString(plc *iampolicyv1.IamPolicy) (results string) {
	result := "ComplianceState is still undetermined"
	if plc.Status.ComplianceState == "" {
		return result
	}
	result = string(plc.Status.ComplianceState)

	if plc.Status.CompliancyDetails == nil {
		return result
	}
	if _, ok := plc.Status.CompliancyDetails[plc.Name]; !ok {
		return result

	}
	if result == "NonCompliant" {
		for _, v := range plc.Status.CompliancyDetails[plc.Name] {
			var complianceStatus = !strings.Contains(fmt.Sprint(v), "0")
			if complianceStatus == true {
				result += fmt.Sprintf("; %s", strings.Join(v, ", "))
			}
		}
	} else {
		for _, v := range plc.Status.CompliancyDetails[plc.Name] {
			result += fmt.Sprintf("; %s", strings.Join(v, ", "))
		}
	}
	return result
}

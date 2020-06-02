// Licensed Materials - Property of IBM
// (c) Copyright IBM Corporation 2018. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Copyright (c) 2020 Red Hat, Inc.

package grcpolicy

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/golang/glog"
	policiesv1 "github.com/open-cluster-management/iam-policy-controller/pkg/apis/iam.policies/v1"
	"github.com/open-cluster-management/iam-policy-controller/pkg/common"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

//=================================================================
// convertPolicyStatusToString to be able to pass the status as event
func convertPolicyStatusToString(plc *policiesv1.IamPolicy) (results string) {
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

func createGenericObjectEvent(name, namespace string) {

	plc := &policiesv1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Policy",
			APIVersion: "policy.open-cluster-management.io/v1",
		},
	}
	data, err := json.Marshal(plc)
	if err != nil {
		glog.Fatal(err)
	}
	found, err := common.GetGenericObject(data, namespace)
	if err != nil {
		glog.Fatal(err)
	}
	if md, ok := found.Object["metadata"]; ok {
		metadata := md.(map[string]interface{})
		if objectUID, ok := metadata["uid"]; ok {
			plc.ObjectMeta.UID = types.UID(objectUID.(string))
			reconcilingAgent.recorder.Event(plc, corev1.EventTypeWarning, "reporting --> forward", fmt.Sprintf("eventing on policy %s/%s", plc.Namespace, plc.Name))
		} else {
			glog.Errorf("the objectUID is missing from policy %s/%s", plc.Namespace, plc.Name)
			return
		}
	}
}

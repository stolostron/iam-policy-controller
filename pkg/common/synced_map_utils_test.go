// Licensed Materials - Property of IBM
// (c) Copyright IBM Corporation 2018. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

// Package admissionpolicy handles admissionpolicy controller logic
package common

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	iampolicyv1 "open-cluster-management.io/iam-policy-controller/api/v1"
)

var plc = &iampolicyv1.IamPolicy{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "testPolicy",
		Namespace: "default",
	},
	Spec: iampolicyv1.IamPolicySpec{
		RemediationAction: iampolicyv1.Inform,
		NamespaceSelector: iampolicyv1.Target{
			Include: []iampolicyv1.NonEmptyString{"default"},
			Exclude: []iampolicyv1.NonEmptyString{"kube*"},
		},
	},
}

var sm = SyncedPolicyMap{
	PolicyMap: make(map[string]*iampolicyv1.IamPolicy),
}

// TestGetObject testing get object in map
func TestGetObject(t *testing.T) {
	_, found := sm.GetObject("void")
	if found {
		t.Fatalf("expecting found = false, however found = %v", found)
	}

	sm.AddObject("default", plc)

	plc, found := sm.GetObject("default")
	if !found {
		t.Fatalf("expecting found = true, however found = %v", found)
	}

	if !reflect.DeepEqual(plc.Name, "testPolicy") {
		t.Fatalf("expecting plcName = testPolicy, however plcName = %v", plc.Name)
	}
}

func TestAddObject(t *testing.T) {
	sm.AddObject("default", plc)
	plcName, _ := sm.GetObject("ServiceInstance")

	_, found := sm.GetObject("void")
	if found {
		t.Fatalf("expecting found = false, however found = %v", found)
	}

	if !reflect.DeepEqual(plc.Name, "testPolicy") {
		t.Fatalf("expecting plcName = testPolicy, however plcName = %v", plcName)
	}
}

func TestRemoveDataObject(t *testing.T) {
	sm.RemoveObject("void")

	_, found := sm.GetObject("void")
	if found {
		t.Fatalf("expecting found = false, however found = %v", found)
	}
	// remove after adding
	sm.AddObject("default", plc)

	sm.RemoveObject("default")

	_, found = sm.GetObject("default")
	if found {
		t.Fatalf("expecting found = false, however found = %v", found)
	}
}

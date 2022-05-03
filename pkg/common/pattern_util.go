// Licensed Materials - Property of IBM
// (c) Copyright IBM Corporation 2018, 2019. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package common

import (
	iampolicyv1 "open-cluster-management.io/iam-policy-controller/api/v1"
)

// ExtractNamespaceLabel to find out the cluster-namespace from the label
func ExtractNamespaceLabel(instance *iampolicyv1.IamPolicy) string {
	if instance.ObjectMeta.Labels == nil {
		return ""
	}

	if _, ok := instance.ObjectMeta.Labels["policy.open-cluster-management.io/cluster-namespace"]; ok {
		return instance.ObjectMeta.Labels["policy.open-cluster-management.io/cluster-namespace"]
	}

	return ""
}

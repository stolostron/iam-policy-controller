// Licensed Materials - Property of IBM
// (c) Copyright IBM Corporation 2018. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// A custom type is required since there is no way to have a kubebuilder marker
// apply to the items of a slice.

// +kubebuilder:validation:MinLength=1
type NonEmptyString string

/// RemediationAction : enforce or inform
// +kubebuilder:validation:Enum=Inform;inform
type RemediationAction string

const (
	// Inform is an remediationAction to only inform
	Inform RemediationAction = "Inform"
)

// ComplianceState shows the state of enforcement
type ComplianceState string

const (
	// Compliant is an ComplianceState
	Compliant ComplianceState = "Compliant"

	// NonCompliant is an ComplianceState
	NonCompliant ComplianceState = "NonCompliant"

	// UnknownCompliancy is an ComplianceState
	UnknownCompliancy ComplianceState = "UnknownCompliancy"
)

// Target defines the list of namespaces to include/exclude
type Target struct {
	Include []NonEmptyString `json:"include,omitempty"`
	Exclude []NonEmptyString `json:"exclude,omitempty"`
}

// IamPolicySpec defines the desired state of IamPolicy
type IamPolicySpec struct {
	// A list of regex values signifying which cluster role binding names to ignore.
	// By default, all cluster role bindings that have a name which starts with system:
	// will be ignored. It is recommended to set this to a stricter value.
	IgnoreClusterRoleBindings []NonEmptyString `json:"ignoreClusterRoleBindings,omitempty"`
	// enforce, inform
	RemediationAction RemediationAction `json:"remediationAction,omitempty"`
	// Selecting a list of namespaces where the policy applies
	NamespaceSelector Target            `json:"namespaceSelector,omitempty"`
	LabelSelector     map[string]string `json:"labelSelector,omitempty"`
	// Maximum number of cluster role binding users still valid before it is considered non-compliant
	MaxClusterRoleBindingUsers int `json:"maxClusterRoleBindingUsers,omitempty"`
	// Name of the cluster role  refefenced by the cluster role bindings,  defaults to "cluster-admin" if none specified
	ClusterRole string `json:"clusterRole,omitempty"`

	// low, medium, or high
	Severity string `json:"severity,omitempty"`
}

type CompliancyDetail map[string][]string

// IamPolicyStatus defines the observed state of IamPolicy
type IamPolicyStatus struct {
	ComplianceState   ComplianceState             `json:"compliant,omitempty"`         // Compliant, NonCompliant, UnknownCompliancy
	CompliancyDetails map[string]CompliancyDetail `json:"compliancyDetails,omitempty"` // reason for non-compliancy
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// IamPolicy is the Schema for the iampolicies API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=iampolicies,scope=Namespaced
type IamPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IamPolicySpec   `json:"spec,omitempty"`
	Status IamPolicyStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// IamPolicyList contains a list of IamPolicy
type IamPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IamPolicy `json:"items"`
}

// Policy is a specification for a Policy resource
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient.
type Policy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
}

// PolicyList is a list of Policy resources
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:lister-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object.
type PolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Policy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IamPolicy{}, &IamPolicyList{})
}

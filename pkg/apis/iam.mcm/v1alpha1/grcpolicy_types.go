// Licensed Materials - Property of IBM
// (c) Copyright IBM Corporation 2018. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RemediationAction : enforce or inform
type RemediationAction string

const (
	// Enforce is an remediationAction to make changes
	Enforce RemediationAction = "Enforce"

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
	Include []string `json:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
}

// IamPolicySpec defines the desired state of GRCPolicy
type IamPolicySpec struct {
	RemediationAction               RemediationAction `json:"remediationAction,omitempty"` //enforce, inform
	NamespaceSelector               Target            `json:"namespaceSelector,omitempty"` // selecting a list of namespaces where the policy applies
	LabelSelector                   map[string]string `json:"labelSelector,omitempty"`
	MaxRoleBindingUsersPerNamespace int               `json:"maxRoleBindingUsersPerNamespace,omitempty"`
	MaxClusterRoleBindingUsers      int               `json:"maxClusterRoleBindingUsers,omitempty"`
}

// IamPolicyStatus defines the observed state of IamPolicy
type IamPolicyStatus struct {
	ComplianceState   ComplianceState                `json:"compliant,omitempty"`         // Compliant, NonCompliant, UnkownCompliancy
	CompliancyDetails map[string]map[string][]string `json:"compliancyDetails,omitempty"` // reason for non-compliancy
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// IamPolicy is the Schema for the policies API
// +k8s:openapi-gen=true
type IamPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IamPolicySpec   `json:"spec,omitempty"`
	Status IamPolicyStatus `json:"status,omitempty"`
}

// Policy is a specification for a Policy resource
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient
type Policy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// IamPolicyList contains a list of IamPolicy
type IamPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IamPolicy `json:"items"`
}

// PolicyList is a list of Policy resources
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:lister-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type PolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Policy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IamPolicy{}, &IamPolicyList{})
	SchemeBuilder.Register(&Policy{}, &PolicyList{})
}

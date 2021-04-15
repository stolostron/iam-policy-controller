// Licensed Materials - Property of IBM
// (c) Copyright IBM Corporation 2018. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package iampolicy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	coretypes "k8s.io/api/core/v1"
	sub "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	testclient "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	policiesv1 "github.com/open-cluster-management/iam-policy-controller/pkg/apis/policy/v1"
	"github.com/open-cluster-management/iam-policy-controller/pkg/common"
)

var mgr manager.Manager
var err error

func TestInitialize(t *testing.T) {
	result := Initialize(nil, nil, "test", "test", "test")
	assert.Nil(t, result)
}

func TestReconcile(t *testing.T) {
	var (
		name      = "foo"
		namespace = "default"
	)
	instance := &policiesv1.IamPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: policiesv1.IamPolicySpec{
			MaxClusterRoleBindingUsers: 1,
		},
	}

	// Objects to track in the fake client.
	objs := []runtime.Object{instance}
	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(policiesv1.SchemeGroupVersion, instance)

	// Create a fake client to mock API calls.
	cl := fake.NewFakeClient(objs...)
	// Create a ReconcileIamPolicy object with the scheme and fake client
	r := &ReconcileIamPolicy{client: cl, scheme: s, recorder: nil}

	// Mock request to simulate Reconcile() being called on an event for a
	// watched resource .
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}
	var simpleClient kubernetes.Interface = testclient.NewSimpleClientset()
	common.Initialize(&simpleClient, nil)
	res, err := r.Reconcile(req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	t.Log(res)
}

func TestPeriodicallyExecIamPolicies(t *testing.T) {
	var (
		name      = "foo"
		namespace = "default"
	)
	var typeMeta = metav1.TypeMeta{
		Kind: "namespace",
	}
	var objMeta = metav1.ObjectMeta{
		Name: "default",
	}
	var ns = coretypes.Namespace{
		TypeMeta:   typeMeta,
		ObjectMeta: objMeta,
	}
	// Mock request to simulate Reconcile() being called on an event for a
	// watched resource .
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}
	instance := &policiesv1.IamPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: policiesv1.IamPolicySpec{
			MaxClusterRoleBindingUsers: 1,
		},
	}

	// Objects to track in the fake client.
	objs := []runtime.Object{instance}
	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(policiesv1.SchemeGroupVersion, instance)

	// Create a fake client to mock API calls.
	cl := fake.NewFakeClient(objs...)

	// Create a ReconcileIamPolicy object with the scheme and fake client.
	r := &ReconcileIamPolicy{client: cl, scheme: s, recorder: nil}
	var simpleClient kubernetes.Interface = testclient.NewSimpleClientset()
	simpleClient.CoreV1().Namespaces().Create(context.TODO(), &ns, metav1.CreateOptions{})
	common.Initialize(&simpleClient, nil)
	res, err := r.Reconcile(req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	t.Log(res)
	var target = []string{"default"}
	iamPolicy.Spec.NamespaceSelector.Include = target
	handleAddingPolicy(&iamPolicy)
	exitExecLoop = "true"
	PeriodicallyExecIamPolicies(1)
}

func TestCheckUnNamespacedPolicies(t *testing.T) {
	var simpleClient kubernetes.Interface = testclient.NewSimpleClientset()
	common.Initialize(&simpleClient, nil)
	var iamPolicy = policiesv1.IamPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		}}

	var policies = map[string]*policiesv1.IamPolicy{}
	policies["policy1"] = &iamPolicy

	_, err := checkUnNamespacedPolicies(policies)
	assert.Nil(t, err)
}

func TestEnsureDefaultLabel(t *testing.T) {
	updateNeeded := ensureDefaultLabel(&iamPolicy)
	assert.True(t, updateNeeded)

	var labels1 = map[string]string{}
	labels1["category"] = grcCategory
	iamPolicy.Labels = labels1
	updateNeeded = ensureDefaultLabel(&iamPolicy)
	assert.False(t, updateNeeded)

	var labels2 = map[string]string{}
	labels2["category"] = "foo"
	iamPolicy.Labels = labels2
	updateNeeded = ensureDefaultLabel(&iamPolicy)
	assert.True(t, updateNeeded)

	var labels3 = map[string]string{}
	labels3["foo"] = grcCategory
	iamPolicy.Labels = labels3
	updateNeeded = ensureDefaultLabel(&iamPolicy)
	assert.True(t, updateNeeded)
}

func TestCheckAllClusterLevel(t *testing.T) {
	var subject = sub.Subject{
		APIGroup:  "",
		Kind:      "User",
		Name:      "user1",
		Namespace: "default",
	}
	var subjects = []sub.Subject{}
	subjects = append(subjects, subject)
	var clusterRoleBinding = sub.ClusterRoleBinding{
		Subjects: subjects,
	}
	var items = []sub.ClusterRoleBinding{}
	items = append(items, clusterRoleBinding)
	var clusterRoleBindingList = sub.ClusterRoleBindingList{
		Items: items,
	}
	var users = checkAllClusterLevel(&clusterRoleBindingList)
	assert.Equal(t, 1, users)
}

func TestPrintMap(t *testing.T) {
	var policies = map[string]*policiesv1.IamPolicy{}
	policies["policy1"] = &iamPolicy
	printMap(policies)
}

func TestCreateParentPolicy(t *testing.T) {
	var ownerReference = metav1.OwnerReference{
		Name: "foo",
	}
	var ownerReferences = []metav1.OwnerReference{}
	ownerReferences = append(ownerReferences, ownerReference)
	iamPolicy.OwnerReferences = ownerReferences

	policy := createParentPolicy(&iamPolicy)
	assert.NotNil(t, policy)
	createParentPolicyEvent(&iamPolicy)
}

func TestHandleAddingPolicy(t *testing.T) {
	var simpleClient kubernetes.Interface = testclient.NewSimpleClientset()
	var typeMeta = metav1.TypeMeta{
		Kind: "namespace",
	}
	var objMeta = metav1.ObjectMeta{
		Name: "default",
	}
	var ns = coretypes.Namespace{
		TypeMeta:   typeMeta,
		ObjectMeta: objMeta,
	}
	simpleClient.CoreV1().Namespaces().Create(context.TODO(), &ns, metav1.CreateOptions{})
	common.Initialize(&simpleClient, nil)
	handleAddingPolicy(&iamPolicy)
	policy, found := availablePolicies.GetObject(iamPolicy.Namespace + "." + iamPolicy.Name)
	assert.True(t, found)
	assert.NotNil(t, policy)
	handleRemovingPolicy("foo", "default")
	policy, found = availablePolicies.GetObject(iamPolicy.Namespace + "." + iamPolicy.Name)
	assert.False(t, found)
	assert.Nil(t, policy)
}

func TestGetContainerID(t *testing.T) {
	var containerStateWaiting = coretypes.ContainerStateWaiting{
		Reason: "unknown",
	}
	var containerState = coretypes.ContainerState{
		Waiting: &containerStateWaiting,
	}
	var containerStatus = coretypes.ContainerStatus{
		State:       containerState,
		ContainerID: "id",
	}
	var containerStatuses []coretypes.ContainerStatus
	containerStatuses = append(containerStatuses, containerStatus)
	var podStatus = coretypes.PodStatus{
		ContainerStatuses: containerStatuses,
	}
	var pod = coretypes.Pod{
		Status: podStatus,
	}
	getContainerID(pod, "foo")
}

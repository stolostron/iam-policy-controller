// Licensed Materials - Property of IBM
// (c) Copyright IBM Corporation 2018. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Copyright (c) 2020 Red Hat, Inc.

package grcpolicy

import (
	"context"
	"log"
	"net/http"
	"reflect"

	"fmt"
	"strings"
	"time"

	policiesv1 "github.com/open-cluster-management/iam-policy-controller/pkg/apis/iam.policies/v1"
	"github.com/open-cluster-management/iam-policy-controller/pkg/common"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	//logf "sigs.k8s.io/controller-runtime/pkg/runtime/log" // yet another logger...
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// Finalizer used to ensure consistency when deleting a CRD
const Finalizer = "finalizer.mcm.ibm.com"

const grcCategory = "system-and-information-integrity"

var clusterName = "managedCluster"

// availablePolicies is a cach all all available polices
var availablePolicies common.SyncedPolicyMap

// PlcChan a channel used to pass policies ready for update
var PlcChan chan *policiesv1.IamPolicy

// KubeClient a k8s client used for k8s native resources
var KubeClient *kubernetes.Clientset

var reconcilingAgent *ReconcileGRCPolicy

// NamespaceWatched defines which namespace we can watch for the GRC policies and ignore others
var NamespaceWatched string

// EventOnParent specifies if we also want to send events to the parent policy. Available options are yes/no/ifpresent
var EventOnParent string

// PrometheusAddr port addr for prom metrics
var PrometheusAddr string

var rpcDurations = prometheus.NewSummaryVec(
	prometheus.SummaryOpts{
		Name:       "policy_processing_durations_milliseconds",
		Help:       "GRC Controller processing latency distributions.",
		Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
	},
	[]string{"policy_processing"},
)

func startProm() {
	// Register the summary with Prometheus's default registry.
	prometheus.MustRegister(rpcDurations)
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(PrometheusAddr, nil))
}

// Initialize to initialize some controller varaibles
func Initialize(kClient *kubernetes.Clientset, mgr manager.Manager, clsName, namespace, eventParent, promAddr string) (err error) {
	KubeClient = kClient
	PlcChan = make(chan *policiesv1.IamPolicy, 100) //buffering up to 100 policies for update

	if clsName != "" {
		clusterName = clsName
	}
	NamespaceWatched = namespace

	EventOnParent = strings.ToLower(eventParent)

	PrometheusAddr = promAddr

	go startProm()

	return nil
}

// Add creates a new IamPolicy Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileGRCPolicy{
		Client:   mgr.GetClient(),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetRecorder("iampolicy-controller"),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("iampolicy-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to IamPolicy
	err = c.Watch(&source.Kind{Type: &policiesv1.IamPolicy{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Uncomment watch a Deployment created by IamPolicy - change this for objects you create
	err = c.Watch(&source.Kind{Type: &policiesv1.IamPolicy{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &policiesv1.IamPolicy{},
	})
	if err != nil {
		return err
	}
	return nil
}

var _ reconcile.Reconciler = &ReconcileGRCPolicy{}

// Annotation for generating RBAC role for writing Events
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// ReconcileGRCPolicy reconciles a IamPolicy object
type ReconcileGRCPolicy struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// Reconcile reads that state of the cluster for a IamPolicy object and makes changes based on the state read
// and what is in the IamPolicy.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=policy.open-cluster-management.io,resources=policies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy.open-cluster-management.io,resources=policies/status,verbs=get;update;patch
func (r *ReconcileGRCPolicy) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the IamPolicy instance
	instance := &policiesv1.IamPolicy{}
	if reconcilingAgent == nil {
		reconcilingAgent = r
	}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		updateNeeded := false
		if !ensureDefaultLabel(instance) {
			updateNeeded = true
		}
		if updateNeeded {
			if err := r.Update(context.Background(), instance); err != nil {
				return reconcile.Result{Requeue: true}, nil
			}
		}
		instance.Status.CompliancyDetails = nil //reset CompliancyDetails
		handleAddingPolicy(instance)
	} else {
		handleRemovingPolicy(instance)
		// The object is being deleted
		return reconcile.Result{}, nil
	}
	glog.V(3).Infof("reason: successful processing, subject: policy/%v, namespace: %v, according to policy: %v, additional-info: none\n", instance.Name, instance.Namespace, instance.Name)
	return reconcile.Result{}, nil
}

func ensureDefaultLabel(instance *policiesv1.IamPolicy) (updateNeeded bool) {
	//we need to ensure this label exists -> category: "System and Information Integrity"
	if instance.ObjectMeta.Labels == nil {
		newlbl := make(map[string]string)
		newlbl["category"] = grcCategory
		instance.ObjectMeta.Labels = newlbl
		return true
	}
	if _, ok := instance.ObjectMeta.Labels["category"]; !ok {
		instance.ObjectMeta.Labels["category"] = grcCategory
		return true
	}
	if instance.ObjectMeta.Labels["category"] != grcCategory {
		instance.ObjectMeta.Labels["category"] = grcCategory
		return true
	}
	return false
}

// PeriodicallyExecGRCPolicies always check status - let this be the only function in the controller
func PeriodicallyExecGRCPolicies(freq uint) {
	var plcToUpdateMap map[string]*policiesv1.IamPolicy
	for {
		start := time.Now()
		printMap(availablePolicies.PolicyMap)
		plcToUpdateMap = make(map[string]*policiesv1.IamPolicy)

		// Loops through all of the iam policies
		for namespace, policy := range availablePolicies.PolicyMap {
			//For each namespace, fetch all the RoleBindings in that NS according to the policy selector
			//For each RoleBindings get the number of users
			//update the status internal map
			//no difference between enforce and inform here

			roleBindingList, err := common.KubeClient.Rbac().RoleBindings(namespace).List(metav1.ListOptions{LabelSelector: labels.Set(policy.Spec.LabelSelector).String()})
			if err != nil {
				glog.Errorf("reason: communication error, subject: k8s API server, namespace: %v, according to policy: %v, additional-info: %v\n", namespace, policy.Name, err)
				continue
			}

			roleBindingViolationCount, violatedBindings := checkRoleBindingViolations(roleBindingList, policy, namespace)
			if strings.ToLower(string(policy.Spec.RemediationAction)) == strings.ToLower(string(policiesv1.Enforce)) {
				glog.V(5).Infof("Enforce is set, but ignored :-)")
			}
			if addRoleBindingsViolationCount(policy, roleBindingViolationCount, violatedBindings, namespace) {
				plcToUpdateMap[policy.Name] = policy
			}
			checkComplianceBasedOnDetails(policy)
		}
		checkUnNamespacedPolicies(plcToUpdateMap)

		//update status of all policies that changed:
		faultyPlc, err := updatePolicyStatus(plcToUpdateMap)
		if err != nil {
			glog.Errorf("reason: policy update error, subject: policy/%v, namespace: %v, according to policy: %v, additional-info: %v\n", faultyPlc.Name, faultyPlc.Namespace, faultyPlc.Name, err)
		}

		//prometheus quantiles for processing delay in each cycle
		elapsed := time.Since(start)
		millis := elapsed / 1000000
		rpcDurations.WithLabelValues("controller").Observe(common.ToFixed(float64(millis), 2))

		//making sure that if processing is > freq we don't sleep
		//if freq > processing we sleep for the remaining duration
		elapsed = time.Since(start) / 1000000000 // convert to seconds
		if float64(freq) > float64(elapsed) {
			remainingSleep := float64(freq) - float64(elapsed)
			time.Sleep(time.Duration(remainingSleep) * time.Second)
		}
	}
}

func checkUnNamespacedPolicies(plcToUpdateMap map[string]*policiesv1.IamPolicy) error {
	plcMap := convertMaptoPolicyNameKey()

	// group the policies with cluster users and the ones with groups
	// take the plc with min users and groups and make it your baseline

	ClusteRoleBindingList, err := common.KubeClient.Rbac().ClusterRoleBindings().List(metav1.ListOptions{})
	if err != nil {
		glog.Errorf("reason: communication error, subject: k8s API server, namespace: all, according to policy: none, additional-info: %v\n", err)
		return err
	}

	clusterLevelUsers := checkAllClusterLevel(ClusteRoleBindingList)
	for _, policy := range plcMap {
		var userViolationCount int
		if policy.Spec.MaxClusterRoleBindingUsers < clusterLevelUsers && policy.Spec.MaxClusterRoleBindingUsers >= 0 {
			userViolationCount = clusterLevelUsers - policy.Spec.MaxClusterRoleBindingUsers
		}
		if addViolationCount(policy, userViolationCount, "cluster-wide") {
			plcToUpdateMap[policy.Name] = policy
		}
		checkComplianceBasedOnDetails(policy)
	}
	return nil
}

func checkAllClusterLevel(clusterRoleBindingList *v1.ClusterRoleBindingList) (userV int) {

	usersMap := make(map[string]bool)
	for _, clusterRoleBinding := range clusterRoleBindingList.Items {
		for _, subject := range clusterRoleBinding.Subjects {
			if subject.Kind == "User" && !strings.HasPrefix(clusterRoleBinding.Name, "system") {
				usersMap[subject.Name] = true
			}
		}
	}
	return len(usersMap)
}

func convertMaptoPolicyNameKey() map[string]*policiesv1.IamPolicy {
	plcMap := make(map[string]*policiesv1.IamPolicy)
	for _, policy := range availablePolicies.PolicyMap {
		plcMap[policy.Name] = policy
	}
	return plcMap
}

func checkRoleBindingViolations(roleBindingList *v1.RoleBindingList, plc *policiesv1.IamPolicy, namespace string) (roleBindingV int, violatedRoleBindings []string) {

	roleBindingsMap := make(map[string]bool)
	for _, roleBinding := range roleBindingList.Items {
		for _, subject := range roleBinding.Subjects {
			if subject.Kind == "Group" && subject.Name != roleBinding.Name && strings.HasPrefix(roleBinding.Name, "icp:") {
				roleBindingsMap[roleBinding.Name] = true
				fmt.Println("violated roleBinding:", roleBinding.Name)
				violatedRoleBindings = append(violatedRoleBindings, roleBinding.Name)
			}
		}
		if !strings.HasPrefix(roleBinding.RoleRef.Name, "icp:") && strings.HasPrefix(roleBinding.Name, "icp:") {
			roleBindingsMap[roleBinding.Name] = true
			violatedRoleBindings = append(violatedRoleBindings, roleBinding.Name)
		}
	}
	var rBindingViolationCount int
	if plc.Spec.MaxRoleBindingViolationsPerNamespace < len(roleBindingsMap) && plc.Spec.MaxRoleBindingViolationsPerNamespace >= 0 {
		rBindingViolationCount = (len(roleBindingsMap) - plc.Spec.MaxRoleBindingViolationsPerNamespace)
	}
	return rBindingViolationCount, violatedRoleBindings
}

func addRoleBindingsViolationCount(plc *policiesv1.IamPolicy, roleBindingCount int, vRoleBindings []string, namespace string) bool {

	changed := false
	msg := fmt.Sprintf("%s rolebindings violations detected in namespace `%s`, violated rolebinding : %v", fmt.Sprint(roleBindingCount), namespace, vRoleBindings)

	if plc.Status.CompliancyDetails == nil {
		plc.Status.CompliancyDetails = make(map[string]map[string][]string)
	}
	if _, ok := plc.Status.CompliancyDetails[plc.Name]; !ok {
		plc.Status.CompliancyDetails[plc.Name] = make(map[string][]string)

	}
	if plc.Status.CompliancyDetails[plc.Name][namespace] == nil {
		plc.Status.CompliancyDetails[plc.Name][namespace] = []string{}
	}
	if len(plc.Status.CompliancyDetails[plc.Name][namespace]) == 0 {
		plc.Status.CompliancyDetails[plc.Name][namespace] = []string{msg}
		changed = true
		return changed
	}
	firstNum := strings.Split(plc.Status.CompliancyDetails[plc.Name][namespace][0], " ")
	if len(firstNum) > 0 {
		if firstNum[0] == fmt.Sprint(roleBindingCount) {
			return false
		}
		plc.Status.CompliancyDetails[plc.Name][namespace][0] = msg
		changed = true
	}
	return changed
}

func addViolationCount(plc *policiesv1.IamPolicy, userCount int, namespace string) bool {

	changed := false
	msg := fmt.Sprintf("Number of users with clusteradmin role is %s above the specified limit", fmt.Sprint(userCount))
	if plc.Status.CompliancyDetails == nil {
		plc.Status.CompliancyDetails = make(map[string]map[string][]string)
	}
	if _, ok := plc.Status.CompliancyDetails[plc.Name]; !ok {
		plc.Status.CompliancyDetails[plc.Name] = make(map[string][]string)

	}
	if plc.Status.CompliancyDetails[plc.Name][namespace] == nil {
		plc.Status.CompliancyDetails[plc.Name][namespace] = []string{}
	}
	if len(plc.Status.CompliancyDetails[plc.Name][namespace]) == 0 {
		plc.Status.CompliancyDetails[plc.Name][namespace] = []string{msg}
		changed = true
		return changed
	}
	firstNum := strings.Split(plc.Status.CompliancyDetails[plc.Name][namespace][0], " ")
	if len(firstNum) > 0 {
		if firstNum[0] == fmt.Sprint(userCount) {
			return false
		}
	}
	plc.Status.CompliancyDetails[plc.Name][namespace][0] = msg
	changed = true
	return changed
}

func checkComplianceBasedOnDetails(plc *policiesv1.IamPolicy) {
	plc.Status.ComplianceState = policiesv1.Compliant
	if plc.Status.CompliancyDetails == nil {
		return
	}
	if _, ok := plc.Status.CompliancyDetails[plc.Name]; !ok {
		return
	}
	if len(plc.Status.CompliancyDetails[plc.Name]) == 0 {
		return
	}
	for namespace, msgList := range plc.Status.CompliancyDetails[plc.Name] {
		if len(msgList) > 0 {
			violationNum := strings.Split(plc.Status.CompliancyDetails[plc.Name][namespace][0], " ")
			if len(violationNum) > 0 {
				if violationNum[7] != fmt.Sprint(0) && strings.HasPrefix(violationNum[0], "Number") {
					plc.Status.ComplianceState = policiesv1.NonCompliant
				}
				if violationNum[0] != fmt.Sprint(0) && strings.HasPrefix(violationNum[1], "rolebindings") {
					plc.Status.ComplianceState = policiesv1.NonCompliant
				}
			}
		} else {
			return
		}
	}
}

func checkComplianceChangeBasedOnDetails(plc *policiesv1.IamPolicy) (complianceChanged bool) {
	//used in case we also want to know not just the compliance state, but also whether the compliance changed or not.
	previous := plc.Status.ComplianceState
	if plc.Status.CompliancyDetails == nil {
		plc.Status.ComplianceState = policiesv1.UnknownCompliancy
		return reflect.DeepEqual(previous, plc.Status.ComplianceState)
	}
	if _, ok := plc.Status.CompliancyDetails[plc.Name]; !ok {
		plc.Status.ComplianceState = policiesv1.UnknownCompliancy
		return reflect.DeepEqual(previous, plc.Status.ComplianceState)
	}
	if len(plc.Status.CompliancyDetails[plc.Name]) == 0 {
		plc.Status.ComplianceState = policiesv1.UnknownCompliancy
		return reflect.DeepEqual(previous, plc.Status.ComplianceState)
	}
	plc.Status.ComplianceState = policiesv1.Compliant
	for namespace, msgList := range plc.Status.CompliancyDetails[plc.Name] {
		if len(msgList) > 0 {
			violationNum := strings.Split(plc.Status.CompliancyDetails[plc.Name][namespace][0], " ")
			if len(violationNum) > 0 {
				if violationNum[0] != fmt.Sprint(0) {
					plc.Status.ComplianceState = policiesv1.NonCompliant
				}
			}
		} else {
			return reflect.DeepEqual(previous, plc.Status.ComplianceState)
		}
	}
	if plc.Status.ComplianceState != policiesv1.NonCompliant {
		plc.Status.ComplianceState = policiesv1.Compliant
	}
	return reflect.DeepEqual(previous, plc.Status.ComplianceState)
}

func updatePolicyStatus(policies map[string]*policiesv1.IamPolicy) (*policiesv1.IamPolicy, error) {
	for _, instance := range policies { // policies is a map where: key = plc.Name, value = pointer to plc
		err := reconcilingAgent.Update(context.TODO(), instance)
		if err != nil {
			return instance, err
		}
		if EventOnParent != "no" {
			createParentPolicyEvent(instance)
		}
		{ //TODO we can make this eventing enabled by a flag
			if instance.Status.ComplianceState == policiesv1.NonCompliant {
				reconcilingAgent.recorder.Event(instance, corev1.EventTypeWarning, fmt.Sprintf("policy: %s/%s", instance.Namespace, instance.Name), convertPolicyStatusToString(instance))
			} else {
				reconcilingAgent.recorder.Event(instance, corev1.EventTypeNormal, fmt.Sprintf("policy: %s/%s", instance.Namespace, instance.Name), convertPolicyStatusToString(instance))
			}
		}
	}
	return nil, nil
}

func getContainerID(pod corev1.Pod, containerName string) string {
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.Name == containerName {
			return containerStatus.ContainerID
		}
	}
	return ""
}

func handleRemovingPolicy(plc *policiesv1.IamPolicy) {
	for k, v := range availablePolicies.PolicyMap {
		if v.Name == plc.Name {
			availablePolicies.RemoveObject(k)
		}
	}
}

func handleAddingPolicy(plc *policiesv1.IamPolicy) error {

	allNamespaces, err := common.GetAllNamespaces()
	if err != nil {

		glog.Errorf("reason: error fetching the list of available namespaces, subject: K8s API server, namespace: all, according to policy: %v, additional-info: %v\n", plc.Name, err)

		return err
	}
	//clean up that policy from the existing namepsaces, in case the modification is in the namespace selector
	for _, ns := range allNamespaces {
		if policy, found := availablePolicies.GetObject(ns); found {
			if policy.Name == plc.Name {
				availablePolicies.RemoveObject(ns)
			}
		}
	}
	selectedNamespaces := common.GetSelectedNamespaces(plc.Spec.NamespaceSelector.Include, plc.Spec.NamespaceSelector.Exclude, allNamespaces)
	for _, ns := range selectedNamespaces {
		availablePolicies.AddObject(ns, plc)
	}
	return err
}

//=================================================================
//deleteExternalDependency in case the CRD was related to non-k8s resource
func (r *ReconcileGRCPolicy) deleteExternalDependency(instance *policiesv1.IamPolicy) error {
	glog.V(0).Infof("reason: CRD deletion, subject: policy/%v, namespace: %v, according to policy: none, additional-info: none\n", instance.Name, instance.Namespace)
	// Ensure that delete implementation is idempotent and safe to invoke
	// multiple types for same object.
	return nil
}

//=================================================================
// Helper functions to check if a string exists in a slice of strings.
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

//=================================================================
// Helper functions to remove a string from a slice of strings.
func removeString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}

//=================================================================
// Helper functions that pretty prints a map
func printMap(myMap map[string]*policiesv1.IamPolicy) {
	if len(myMap) == 0 {
		fmt.Println("Waiting for iam policies to be available for processing... ")
		return
	}
	fmt.Println("Available iam policies in namespaces: ")
	for k, v := range myMap {
		fmt.Printf("namespace = %v; policy = %v \n", k, v.Name)
	}
}

func createParentPolicyEvent(instance *policiesv1.IamPolicy) {
	if len(instance.OwnerReferences) == 0 {
		return //there is nothing to do, since no owner is set
	}
	// we are making an assumption that the GRC policy has a single owner, or we chose the first owner in the list
	if string(instance.OwnerReferences[0].UID) == "" {
		return //there is nothing to do, since no owner UID is set
	}

	parentPlc := createParentPolicy(instance)

	if instance.Status.ComplianceState == policiesv1.NonCompliant {
		reconcilingAgent.recorder.Event(&parentPlc, corev1.EventTypeWarning, fmt.Sprintf("policy: %s/%s", instance.Namespace, instance.Name), convertPolicyStatusToString(instance))
	} else {
		reconcilingAgent.recorder.Event(&parentPlc, corev1.EventTypeNormal, fmt.Sprintf("policy: %s/%s", instance.Namespace, instance.Name), convertPolicyStatusToString(instance))
	}
}

func createParentPolicy(instance *policiesv1.IamPolicy) policiesv1.Policy {
	ns := common.ExtractNamespaceLabel(instance)
	if ns == "" {
		ns = NamespaceWatched
	}
	plc := policiesv1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.OwnerReferences[0].Name,
			Namespace: ns, // we are making an assumption here that the parent policy is in the watched-namespace passed as flag
			UID:       instance.OwnerReferences[0].UID,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Policy",
			APIVersion: "policy.open-cluster-management.io/v1",
		},
	}
	return plc
}

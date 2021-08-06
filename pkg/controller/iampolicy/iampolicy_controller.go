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
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	policiesv1 "github.com/open-cluster-management/iam-policy-controller/pkg/apis/policy/v1"
	"github.com/open-cluster-management/iam-policy-controller/pkg/common"
	"github.com/operator-framework/operator-sdk/pkg/predicate"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_iampolicy")

// Finalizer used to ensure consistency when deleting a CRD
const Finalizer = "finalizer.mcm.ibm.com"

const grcCategory = "system-and-information-integrity"

var clusterName = "managedCluster"

// availablePolicies is a cach all all available polices
var availablePolicies common.SyncedPolicyMap

// PlcChan a channel used to pass policies ready for update
var PlcChan chan *policiesv1.IamPolicy

// KubeClient a k8s client used for k8s native resources
var KubeClient *kubernetes.Interface

var reconcilingAgent *ReconcileIamPolicy

// NamespaceWatched defines which namespace we can watch for the GRC policies and ignore others
var NamespaceWatched string

// EventOnParent specifies if we also want to send events to the parent policy. Available options are yes/no/ifpresent
var EventOnParent string

var formatString string = "policy: %s/%s"

// A way to allow exiting out of the periodic policy check loop
var exitExecLoop string

// Format string taking the role name and the user count to create the violation message
const violationMsgF = "The number of users with the %s role is %s above the specified limit"

// Format string taking the role name to make the regex to extract the usercount from the violation message
// Reminder: always `regexp.QuoteMeta` the input here.
const violationMsgFUserCountRegex = `^(?:The number of users with the %s role is )(\d+)(?: above the specified limit)$`

// Initialize to initialize some controller varaibles
func Initialize(kClient *kubernetes.Interface, mgr manager.Manager, clsName, namespace,
	eventParent string) (err error) {
	KubeClient = kClient
	PlcChan = make(chan *policiesv1.IamPolicy, 100) //buffering up to 100 policies for update

	if clsName != "" {
		clusterName = clsName
	}
	NamespaceWatched = namespace

	EventOnParent = strings.ToLower(eventParent)

	return nil
}

// Add creates a new IamPolicy Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileIamPolicy{
		client:   mgr.GetClient(),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetEventRecorderFor("iampolicy-controller"),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("iampolicy-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource IamPolicy
	pred := predicate.GenerationChangedPredicate{}
	err = c.Watch(&source.Kind{Type: &policiesv1.IamPolicy{}}, &handler.EnqueueRequestForObject{}, pred)
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileIamPolicy implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileIamPolicy{}

// ReconcileIamPolicy reconciles a IamPolicy object
// Annotation for generating RBAC role for writing Events
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
type ReconcileIamPolicy struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client   client.Client
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
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileIamPolicy) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling IamPolicy")

	if reconcilingAgent == nil {
		reconcilingAgent = r
	}

	// Fetch the IamPolicy instance
	instance := &policiesv1.IamPolicy{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			handleRemovingPolicy(request.NamespacedName.Name, request.NamespacedName.Namespace)
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
			if err := r.client.Update(context.Background(), instance); err != nil {
				return reconcile.Result{Requeue: true}, nil
			}
		}
		instance.Status.CompliancyDetails = nil //reset CompliancyDetails

		reqLogger.Info("Iam policy was found, adding it...")
		handleAddingPolicy(instance)

	}
	reqLogger.Info("Reconcile complete.")
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

// PeriodicallyExecIamPolicies always check status - let this be the only function in the controller
func PeriodicallyExecIamPolicies(freq uint) {
	var plcToUpdateMap map[string]*policiesv1.IamPolicy
	for {
		start := time.Now()
		printMap(availablePolicies.PolicyMap)
		plcToUpdateMap = make(map[string]*policiesv1.IamPolicy)

		//currently no support for perNamespace rolebindings
		update, err := checkUnNamespacedPolicies(plcToUpdateMap)
		if err != nil {
			glog.Errorf("Error checking un-namespaced policies, additional info %v \n", err)
		}

		if update {
			//update status of all policies that changed:
			faultyPlc, err := updatePolicyStatus(plcToUpdateMap)
			if err != nil {
				glog.Errorf("reason: policy update error, subject: policy/%v, namespace: %v, according to "+
					"policy: %v, additional-info: %v\n", faultyPlc.Name, faultyPlc.Namespace, faultyPlc.Name, err)
			}
		}

		//check if continue
		if exitExecLoop == "true" {
			return
		}
		//making sure that if processing is > freq we don't sleep
		//if freq > processing we sleep for the remaining duration
		elapsed := time.Since(start) / 1000000000 // convert to seconds
		if float64(freq) > float64(elapsed) {
			remainingSleep := float64(freq) - float64(elapsed)
			time.Sleep(time.Duration(remainingSleep) * time.Second)
		}
	}
}

func checkUnNamespacedPolicies(plcToUpdateMap map[string]*policiesv1.IamPolicy) (bool, error) {
	plcMap := convertMaptoPolicyNameKey()

	// group the policies with cluster users and the ones with groups
	// take the plc with min users and groups and make it your baseline

	ClusteRoleBindingList, err := (*common.KubeClient).RbacV1().ClusterRoleBindings().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		glog.Errorf("reason: communication error, subject: k8s API server, namespace: all, "+
			"according to policy: none, additional-info: %v\n", err)
		return false, err
	}

	update := false
	for _, policy := range plcMap {
		var userViolationCount int

		clusterRoleRef := "cluster-admin"
		if policy.Spec.ClusterRole != "" {
			clusterRoleRef = policy.Spec.ClusterRole
		}
		clusterLevelUsers := checkAllClusterLevel(ClusteRoleBindingList, clusterRoleRef)

		if policy.Spec.MaxClusterRoleBindingUsers < clusterLevelUsers && policy.Spec.MaxClusterRoleBindingUsers >= 0 {
			userViolationCount = clusterLevelUsers - policy.Spec.MaxClusterRoleBindingUsers
		}
		if addViolationCount(policy, clusterRoleRef, userViolationCount, "cluster-wide") {
			plcToUpdateMap[policy.Name] = policy
			update = true
		}
		checkComplianceBasedOnDetails(policy, clusterRoleRef)
	}
	return update, nil
}

func checkAllClusterLevel(clusterRoleBindingList *v1.ClusterRoleBindingList, clusterroleref string) (userV int) {

	usersMap := make(map[string]bool)
	for _, clusterRoleBinding := range clusterRoleBindingList.Items {
		//if not system binding
		if !strings.HasPrefix(clusterRoleBinding.Name, "system") {
			//Only consider role bindings with matching referenced cluster role
			var roleRef = clusterRoleBinding.RoleRef
			if roleRef.Kind == "ClusterRole" && roleRef.Name == clusterroleref {
				for _, subject := range clusterRoleBinding.Subjects {
					if subject.Kind == "User" {
						usersMap[subject.Name] = true
					}
				}
			}

		}
	}
	return len(usersMap)
}

func convertMaptoPolicyNameKey() map[string]*policiesv1.IamPolicy {
	plcMap := make(map[string]*policiesv1.IamPolicy)
	for _, policy := range availablePolicies.PolicyMap {
		plcMap[fmt.Sprintf("%s.%s", policy.Namespace, policy.Name)] = policy
	}
	return plcMap
}

func addViolationCount(plc *policiesv1.IamPolicy, roleName string, userCount int, namespace string) (changed bool) {
	msg := fmt.Sprintf(violationMsgF, roleName, fmt.Sprint(userCount))
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
		return true
	}
	oldUserCount, err := extractUserCount(plc.Status.CompliancyDetails[plc.Name][namespace][0], roleName)
	if err == nil && oldUserCount == userCount {
		return false
	}
	plc.Status.CompliancyDetails[plc.Name][namespace][0] = msg
	return true
}

func checkComplianceBasedOnDetails(plc *policiesv1.IamPolicy, roleName string) {
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
		if len(msgList) == 0 {
			return
		}
		userCount, err := extractUserCount(plc.Status.CompliancyDetails[plc.Name][namespace][0], roleName)
		if err == nil && userCount != 0 {
			plc.Status.ComplianceState = policiesv1.NonCompliant
		}
	}
}

func updatePolicyStatus(policies map[string]*policiesv1.IamPolicy) (*policiesv1.IamPolicy, error) {
	for _, instance := range policies { // policies is a map where: key = plc.Name, value = pointer to plc
		err := reconcilingAgent.client.Status().Update(context.TODO(), instance)
		if err != nil {
			return instance, err
		}
		if EventOnParent != "no" {
			createParentPolicyEvent(instance)
		}
		{ // Can we make this eventing enabled by a flag
			if reconcilingAgent.recorder != nil {
				if instance.Status.ComplianceState == policiesv1.NonCompliant {
					reconcilingAgent.recorder.Event(instance, corev1.EventTypeWarning,
						fmt.Sprintf(formatString, instance.Namespace, instance.Name),
						convertPolicyStatusToString(instance))
				} else {
					reconcilingAgent.recorder.Event(instance, corev1.EventTypeNormal,
						fmt.Sprintf(formatString, instance.Namespace, instance.Name),
						convertPolicyStatusToString(instance))
				}
			}
		}
	}
	return nil, nil
}

func extractUserCount(msg, roleName string) (int, error) {
	regexStr := fmt.Sprintf(violationMsgFUserCountRegex, regexp.QuoteMeta(roleName))
	re, err := regexp.Compile(regexStr)
	if err != nil {
		return 0, err
	}
	regexGroups := re.FindStringSubmatch(msg)
	if len(regexGroups) < 1 {
		return 0, fmt.Errorf("could not find userCount in message")
	}
	return strconv.Atoi(regexGroups[1])
}

func getContainerID(pod corev1.Pod, containerName string) string {
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.Name == containerName {
			return containerStatus.ContainerID
		}
	}
	return ""
}

func handleRemovingPolicy(name string, namespace string) {
	for k, v := range availablePolicies.PolicyMap {
		if v.Name == name && v.Namespace == namespace {
			availablePolicies.RemoveObject(k)
		}
	}
}

func handleAddingPolicy(plc *policiesv1.IamPolicy) {

	// Since this policy isn't namespace based it will ignore namespace selection so the cluster is always checked
	availablePolicies.AddObject(fmt.Sprintf("%s.%s", plc.Namespace, plc.Name), plc)
}

//=================================================================
// Helper functions that pretty prints a map
func printMap(myMap map[string]*policiesv1.IamPolicy) {
	if len(myMap) == 0 {
		fmt.Println("Waiting for iam policies to be available for processing... ")
		return
	}
	fmt.Println("Available iam policies: ")
	for _, v := range myMap {
		fmt.Printf("policy = %v \n", v.Name)
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
		reconcilingAgent.recorder.Event(&parentPlc, corev1.EventTypeWarning,
			fmt.Sprintf(formatString, instance.Namespace, instance.Name), convertPolicyStatusToString(instance))
	} else {
		reconcilingAgent.recorder.Event(&parentPlc, corev1.EventTypeNormal,
			fmt.Sprintf(formatString, instance.Namespace, instance.Name), convertPolicyStatusToString(instance))
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

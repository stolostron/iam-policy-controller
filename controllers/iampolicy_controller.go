// Licensed Materials - Property of IBM
// (c) Copyright IBM Corporation 2018. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	iampolicyv1 "open-cluster-management.io/iam-policy-controller/api/v1"
	"open-cluster-management.io/iam-policy-controller/pkg/common"
)

const (
	grcCategory = "system-and-information-integrity"
	// Format string taking the role name and the user count to create the violation message
	violationMsgF = "The number of users with the %s role is at least %s above the specified limit"
	// Format string taking the role name to make the regex to extract the usercount from the violation message
	// Reminder: always `regexp.QuoteMeta` the input here.
	violationMsgFUserCountRegex = `^(?:The number of users with the %s role is at least )` +
		`(\d+)(?: above the specified limit)$`
	// The default IgnoreClusterRoleBindings regex when not specified in the policy.
	defaultIgnoreCRBs = `^system:.+$`
	ControllerName    = "iam-policy-controller"
)

var (
	log = ctrl.Log.WithName(ControllerName)
	// blank assignment to verify that ReconcileIamPolicy implements reconcile.Reconciler
	_ reconcile.Reconciler = &IamPolicyReconciler{}
	// ClusterRoleBinding objects in OpenShift set the API group of a Group subject
	// to rbac.authorization.k8s.io even though it should be user.openshift.io.
	openShiftGroupGVR = schema.GroupVersionResource{
		Group:    "user.openshift.io",
		Version:  "v1",
		Resource: "groups",
	}
	// availablePolicies is a cache of all available policies
	availablePolicies common.SyncedPolicyMap
	// PlcChan a channel used to pass policies ready for update
	PlcChan chan *iampolicyv1.IamPolicy
	// KubeClient a k8s client used for k8s native resources
	KubeClient *kubernetes.Interface
	// KubeDynamicClient a dynamic k8s client
	KubeDynamicClient *dynamic.Interface
	reconcilingAgent  *IamPolicyReconciler
	// NamespaceWatched defines which namespace we can watch for the GRC policies and ignore others
	NamespaceWatched string
	// EventOnParent specifies if we also want to send events to the parent policy. Available options
	// are yes/no/ifpresent
	EventOnParent string
	// Formats the reason section of generated events
	formatString = "policy: %s/%s"
	// A way to allow exiting out of the periodic policy check loop
	exitExecLoop string
)

// Initialize to initialize some controller variables
func Initialize(
	kClient *kubernetes.Interface,
	kDynamicClient *dynamic.Interface,
	mgr manager.Manager,
	clsName,
	namespace,
	eventParent string) {
	KubeClient = kClient
	KubeDynamicClient = kDynamicClient
	PlcChan = make(chan *iampolicyv1.IamPolicy, 100) // buffering up to 100 policies for update

	NamespaceWatched = namespace

	EventOnParent = strings.ToLower(eventParent)
}

// IamPolicyReconciler reconciles a IamPolicy object
// Annotation for generating RBAC role for writing Events
type IamPolicyReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=policy.open-cluster-management.io,resources=iampolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy.open-cluster-management.io,resources=iampolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=policy.open-cluster-management.io,resources=iampolicies/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list
// +kubebuilder:rbac:groups=user.openshift.io,resources=groups,verbs=get

// Reconcile reads that state of the cluster for a IamPolicy object and makes changes based on the state read
// and what is in the IamPolicy.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *IamPolicyReconciler) Reconcile(tx context.Context, request ctrl.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling IamPolicy")

	if reconcilingAgent == nil {
		reconcilingAgent = r
	}

	// Fetch the IamPolicy instance
	instance := &iampolicyv1.IamPolicy{}

	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info("Policy could not be found, removing it")
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
			if err := r.Update(context.Background(), instance); err != nil {
				// return nil here is intentional
				//nolint:nilerr
				return reconcile.Result{Requeue: true}, nil
			}
		}

		instance.Status.CompliancyDetails = nil // reset CompliancyDetails

		reqLogger.Info("Iam policy was found, adding it...")
		handleAddingPolicy(instance)
	}

	reqLogger.Info("Reconcile complete.")

	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IamPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&iampolicyv1.IamPolicy{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}

func ensureDefaultLabel(instance *iampolicyv1.IamPolicy) (updateNeeded bool) {
	// we need to ensure this label exists -> category: "System and Information Integrity"
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
	log.V(3).Info("Entered PeriodicallyExecIamPolicies")
	var plcToUpdateMap map[string]*iampolicyv1.IamPolicy

	for {
		start := time.Now()

		printMap(availablePolicies.PolicyMap)

		plcToUpdateMap = make(map[string]*iampolicyv1.IamPolicy)

		// currently no support for perNamespace rolebindings
		update, err := checkUnNamespacedPolicies(plcToUpdateMap)
		if err != nil {
			log.Error(err, "Error checking un-namespaced policies")
		}

		if update {
			// update status of all policies that changed:
			faultyPlc, err := updatePolicyStatus(plcToUpdateMap)
			if err != nil {
				log.Error(err, "Unable to update policy status",
					"Name", faultyPlc.Name, "Namespace", faultyPlc.Namespace)
			}
		}

		// check if continue
		if exitExecLoop == "true" {
			log.V(3).Info("Exiting PeriodicallyExecIamPolicies")

			return
		}
		// making sure that if processing is > freq we don't sleep
		// if freq > processing we sleep for the remaining duration
		elapsed := time.Since(start) / 1000000000 // convert to seconds
		if float64(freq) > float64(elapsed) {
			remainingSleep := float64(freq) - float64(elapsed)
			time.Sleep(time.Duration(remainingSleep) * time.Second)
		}
	}
}

func checkUnNamespacedPolicies(
	plcToUpdateMap map[string]*iampolicyv1.IamPolicy) (bool, error) {
	plcMap := convertMaptoPolicyNameKey()

	// group the policies with cluster users and the ones with groups
	// take the plc with min users and groups and make it your baseline

	ClusteRoleBindingList, err := (*common.KubeClient).RbacV1().ClusterRoleBindings().List(
		context.TODO(),
		metav1.ListOptions{})
	if err != nil {
		log.Error(err, "Error listing ClusterRoleBindings")

		return false, err
	}

	update := false

	for _, policy := range plcMap {
		var userViolationCount int

		clusterRoleRef := "cluster-admin"
		if policy.Spec.ClusterRole != "" {
			clusterRoleRef = policy.Spec.ClusterRole
		}

		clusterLevelUsers, err := checkAllClusterLevel(
			ClusteRoleBindingList,
			clusterRoleRef,
			policy.Spec.IgnoreClusterRoleBindings,
		)

		queryErrEncountered := false

		if err != nil {
			queryErrEncountered = true

			log.Info("Error listing users bound to ClusterRole.", "Name", policy.Name, "ClusterRole", clusterRoleRef)
		}

		log.Info(fmt.Sprintf("Found %d users bound to ClusterRole.", clusterLevelUsers),
			"Name", policy.Name, "ClusterRole", clusterRoleRef)

		if policy.Spec.MaxClusterRoleBindingUsers < clusterLevelUsers && policy.Spec.MaxClusterRoleBindingUsers >= 0 {
			userViolationCount = clusterLevelUsers - policy.Spec.MaxClusterRoleBindingUsers
		}

		// Handle the case when there was an error getting the whole user list.
		if queryErrEncountered {
			// Even if there was an error getting the whole user list, as long as we know there is
			// a violation, the policy should be updated to non-compliant unless it is already
			// non-compliant. If it's already non-compliant, we don't want to only have the number
			// of users change.
			if userViolationCount > 0 {
				if policy.Status.ComplianceState == iampolicyv1.NonCompliant {
					continue
				}
			} else if policy.Status.ComplianceState != iampolicyv1.Compliant {
				log.Info("Not updating status to compliant due to error listing users.", "Name", policy.Name)

				continue
			}
		}

		if addViolationCount(policy, clusterRoleRef, userViolationCount, "cluster-wide") {
			plcToUpdateMap[policy.Name] = policy
			update = true
		}

		checkComplianceBasedOnDetails(policy, clusterRoleRef)
	}

	return update, nil
}

// getGroupMembership queries for the membership of an OpenShift group. If the group is not found
// or is malformed, and empty string slice is returned. If the query itself failed, an error is
// returned.
func getGroupMembership(group string) ([]string, error) {
	openShiftUserGroup, err := (*KubeDynamicClient).Resource(openShiftGroupGVR).Get(
		context.TODO(),
		group,
		metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info(fmt.Sprintf("The group %s was not found.", group))

			return []string{}, nil
		}

		return nil, fmt.Errorf("failed to get OpenShift group %s: %w", group, err)
	}

	users, found, err := unstructured.NestedStringSlice(openShiftUserGroup.Object, "users")
	if err != nil || !found {
		log.Info(fmt.Sprintf("Could not retrieve users from group '%s'.", group))

		// returning nil here is intentional
		//nolint:nilerr
		return []string{}, nil
	}

	return users, nil
}

func checkAllClusterLevel(
	clusterRoleBindingList *v1.ClusterRoleBindingList,
	clusterroleref string,
	ignoreCRBs []iampolicyv1.NonEmptyString,
) (userV int, err error) {
	if len(ignoreCRBs) == 0 {
		ignoreCRBs = []iampolicyv1.NonEmptyString{defaultIgnoreCRBs}
	}

	compiledIgnoreCRBs := make([]*regexp.Regexp, 0, len(ignoreCRBs))

	for _, regex := range ignoreCRBs {
		regex, err := regexp.Compile(string(regex))
		if err != nil {
			err = fmt.Errorf("ignoreClusterRoleBindings value '%s' is not a valid regular expression: %w", regex, err)

			return 0, err
		}

		compiledIgnoreCRBs = append(compiledIgnoreCRBs, regex)
	}

	usersMap := make(map[string]bool)

	for _, clusterRoleBinding := range clusterRoleBindingList.Items {
		ignore := false

		for _, regex := range compiledIgnoreCRBs {
			if regex.MatchString(clusterRoleBinding.Name) {
				log.Info(fmt.Sprintf("ignoreClusterRoleBinding entry '%s' matched '%s'. Skipping.",
					regex, clusterRoleBinding.Name))

				ignore = true

				break
			}
		}

		if ignore {
			continue
		}

		// Only consider role bindings with matching referenced cluster role
		roleRef := clusterRoleBinding.RoleRef
		if roleRef.Kind == "ClusterRole" && roleRef.Name == clusterroleref {
			for _, subject := range clusterRoleBinding.Subjects {
				if subject.Kind == "User" {
					usersMap[subject.Name] = true
				} else if subject.Kind == "Group" {
					users, err := getGroupMembership(subject.Name)
					if err != nil {
						log.Error(err, "Error retrieving users in group (policy compliance will be unknown)",
							"ClusterRoleBinding", clusterRoleBinding.Name, "ClusterRole", clusterroleref,
							"Group", subject.Name)

						continue
					}

					for _, user := range users {
						usersMap[user] = true
					}
				}
			}
		}
	}

	return len(usersMap), err
}

func convertMaptoPolicyNameKey() map[string]*iampolicyv1.IamPolicy {
	plcMap := make(map[string]*iampolicyv1.IamPolicy)
	for _, policy := range availablePolicies.PolicyMap {
		plcMap[fmt.Sprintf("%s.%s", policy.Namespace, policy.Name)] = policy
	}

	return plcMap
}

func addViolationCount(plc *iampolicyv1.IamPolicy, roleName string, userCount int, namespace string) (changed bool) {
	msg := fmt.Sprintf(violationMsgF, roleName, fmt.Sprint(userCount))

	if plc.Status.CompliancyDetails == nil {
		plc.Status.CompliancyDetails = make(map[string]iampolicyv1.CompliancyDetail)
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

func checkComplianceBasedOnDetails(plc *iampolicyv1.IamPolicy, roleName string) {
	plc.Status.ComplianceState = iampolicyv1.Compliant
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
			plc.Status.ComplianceState = iampolicyv1.NonCompliant
		}
	}
}

func updatePolicyStatus(policies map[string]*iampolicyv1.IamPolicy) (*iampolicyv1.IamPolicy, error) {
	log.Info("Updating status for IAM Policies")

	for _, instance := range policies { // policies is a map where: key = plc.Name, value = pointer to plc
		err := reconcilingAgent.Status().Update(context.TODO(), instance)
		if err != nil {
			return instance, err
		}

		if EventOnParent != "no" {
			createParentPolicyEvent(instance)
		}

		// Can we make this eventing enabled by a flag
		if reconcilingAgent.Recorder != nil {
			if instance.Status.ComplianceState == iampolicyv1.NonCompliant {
				reconcilingAgent.Recorder.Event(instance, corev1.EventTypeWarning,
					fmt.Sprintf(formatString, instance.Namespace, instance.Name),
					convertPolicyStatusToString(instance))
			} else {
				reconcilingAgent.Recorder.Event(instance, corev1.EventTypeNormal,
					fmt.Sprintf(formatString, instance.Namespace, instance.Name),
					convertPolicyStatusToString(instance))
			}
		}

		log.Info("Status update complete", "IAMPolicy", instance.Name)
	}

	// return nil here is intentional
	return nil, nil
}

func extractUserCount(msg, roleName string) (int, error) {
	regexStr := fmt.Sprintf(violationMsgFUserCountRegex, regexp.QuoteMeta(roleName))

	regularExpression, err := regexp.Compile(regexStr)
	if err != nil {
		return 0, err
	}

	regexGroups := regularExpression.FindStringSubmatch(msg)

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

func handleAddingPolicy(plc *iampolicyv1.IamPolicy) {
	// Since this policy isn't namespace based it will ignore namespace selection so the cluster is always checked
	availablePolicies.AddObject(fmt.Sprintf("%s.%s", plc.Namespace, plc.Name), plc)
}

//=================================================================
// Helper functions that pretty prints a map
func printMap(myMap map[string]*iampolicyv1.IamPolicy) {
	if len(myMap) == 0 {
		log.Info("Waiting for iam policies to be available for processing... ")

		return
	}

	log.Info("Available iam policies: ")

	for _, v := range myMap {
		log.Info(fmt.Sprintf("policy = %v", v.Name))
	}
}

func createParentPolicyEvent(instance *iampolicyv1.IamPolicy) {
	if len(instance.OwnerReferences) == 0 {
		return // there is nothing to do, since no owner is set
	}
	// we are making an assumption that the GRC policy has a single owner, or we chose the first owner in the list
	if string(instance.OwnerReferences[0].UID) == "" {
		return // there is nothing to do, since no owner UID is set
	}

	parentPlc := createParentPolicy(instance)

	if instance.Status.ComplianceState == iampolicyv1.NonCompliant {
		reconcilingAgent.Recorder.Event(&parentPlc, corev1.EventTypeWarning,
			fmt.Sprintf(formatString, instance.Namespace, instance.Name), convertPolicyStatusToString(instance))
	} else {
		reconcilingAgent.Recorder.Event(&parentPlc, corev1.EventTypeNormal,
			fmt.Sprintf(formatString, instance.Namespace, instance.Name), convertPolicyStatusToString(instance))
	}
}

func createParentPolicy(instance *iampolicyv1.IamPolicy) policiesv1.Policy {
	namespace := common.ExtractNamespaceLabel(instance)
	if namespace == "" {
		namespace = NamespaceWatched
	}

	plc := policiesv1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name: instance.OwnerReferences[0].Name,
			// we are making an assumption here
			// that the parent policy is in the watched-namespace passed as flag
			Namespace: namespace,
			UID:       instance.OwnerReferences[0].UID,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Policy",
			APIVersion: "policy.open-cluster-management.io/v1",
		},
	}

	return plc
}

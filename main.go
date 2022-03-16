// Licensed Materials - Property of IBM
// (c) Copyright IBM Corporation 2018. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/pflag"
	policiesv1 "github.com/stolostron/governance-policy-propagator/api/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"open-cluster-management.io/addon-framework/pkg/lease"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	iampolicyv1 "github.com/stolostron/iam-policy-controller/api/v1"
	"github.com/stolostron/iam-policy-controller/controllers"
	common "github.com/stolostron/iam-policy-controller/pkg/common"
	"github.com/stolostron/iam-policy-controller/version"
)

var (
	setupLog = ctrl.Log.WithName("setup")
	scheme   = k8sruntime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(policiesv1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
	utilruntime.Must(iampolicyv1.AddToScheme(scheme))
}

func printVersion() {
	setupLog.Info("Using", "OperatorVersion", version.Version, "GoVersion", runtime.Version(),
		"GOOS", runtime.GOOS, "GOARCH", runtime.GOARCH)
}

func main() {
	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine)
	// Add flags registered by imported packages (e.g. glog and controller-runtime)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	var clusterName, eventOnParent, hubConfigSecretNs, hubConfigSecretName, metricsAddr, probeAddr string
	var frequency uint
	var enableLease, enableLeaderElection, legacyLeaderElection bool

	pflag.UintVar(&frequency, "update-frequency", 10, "The status update frequency (in seconds) of a mutation policy")
	pflag.StringVar(
		&eventOnParent,
		"parent-event",
		"ifpresent",
		"to also send status events on parent policy. options are: yes/no/ifpresent")
	pflag.StringVar(&clusterName, "cluster-name", "mcm-managed-cluster", "Name of the cluster")
	pflag.BoolVar(
		&enableLease,
		"enable-lease",
		false,
		"If enabled, the controller will start the lease controller to report its status")
	pflag.StringVar(
		&hubConfigSecretNs,
		"hubconfig-secret-ns",
		"open-cluster-management-agent-addon",
		"Namespace for hub config kube-secret")
	pflag.StringVar(
		&hubConfigSecretName,
		"hubconfig-secret-name",
		"iam-policy-controller-hub-kubeconfig",
		"Name of the hub config kube-secret")
	pflag.BoolVar(&enableLeaderElection, "leader-elect", true,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	pflag.BoolVar(&legacyLeaderElection, "legacy-leader-elect", false,
		"Use a legacy leader election method for controller manager instead of the lease API.")
	pflag.StringVar(&metricsAddr, "metrics-bind-address", ":8383", "The address the metric endpoint binds to.")
	pflag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")

	pflag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	printVersion()

	namespace, err := common.GetWatchNamespace()
	if err != nil {
		setupLog.Error(err, "Failed to get watch namespace")
		os.Exit(1)
	}

	// Set default manager options
	options := ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Namespace:              namespace,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "iam-policy-controller.open-cluster-management.io",
	}

	if strings.Contains(namespace, ",") {
		options.Namespace = ""
		options.NewCache = cache.MultiNamespacedCacheBuilder(strings.Split(namespace, ","))
	}

	if legacyLeaderElection {
		// If legacyLeaderElection is enabled, then that means the lease API is not available.
		// In this case, use the legacy leader election method of a ConfigMap.
		options.LeaderElectionResourceLock = "configmaps"
	}

	// Create a new manager to provide shared dependencies and start components
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controllers.IamPolicyReconciler{
		Client:   mgr.GetClient(),
		Recorder: mgr.GetEventRecorderFor("iampolicy-controller"),
		Scheme:   mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "IamPolicy")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("Registering Components.")

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}

	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	// Initialize some variables
	var generatedClient kubernetes.Interface = kubernetes.NewForConfigOrDie(mgr.GetConfig())

	dynamicClient := dynamic.NewForConfigOrDie(mgr.GetConfig())
	common.Initialize(&generatedClient, mgr.GetConfig())

	controllers.Initialize(&generatedClient, &dynamicClient, mgr, clusterName, namespace, eventOnParent)

	// PeriodicallyExecIamPolicies is the go-routine that periodically checks the policies
	// and does the needed work to make sure the desired state is achieved
	go controllers.PeriodicallyExecIamPolicies(frequency)

	if enableLease {
		operatorNs, err := common.GetOperatorNamespace()
		if err != nil {
			if errors.Is(err, common.ErrNoNamespace) || errors.Is(err, common.ErrRunLocal) {
				setupLog.Info("Skipping lease; not running in a cluster.")
			} else {
				setupLog.Error(err, "Failed to get operator namespace")
				os.Exit(1)
			}
		} else {
			setupLog.Info(fmt.Sprintf("Found operator namespace %s.", operatorNs))
			hubCfg, err := common.LoadHubConfig(hubConfigSecretNs, hubConfigSecretName)
			if err != nil {
				setupLog.Error(err, "Unable to load hub kubeconfig, setting up leaseUpdater anyway")
			}

			setupLog.Info("Starting lease controller to report status")
			leaseUpdater := lease.NewLeaseUpdater(
				generatedClient,
				"iam-policy-controller",
				operatorNs,
			).WithHubLeaseConfig(hubCfg, clusterName)

			go leaseUpdater.Start(context.TODO())
		}
	} else {
		setupLog.Info("Status reporting is not enabled")
	}

	setupLog.Info("starting manager")

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

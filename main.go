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

	"github.com/go-logr/zapr"
	"github.com/spf13/pflag"
	"github.com/stolostron/go-log-utils/zaputil"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"open-cluster-management.io/addon-framework/pkg/lease"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	iampolicyv1 "open-cluster-management.io/iam-policy-controller/api/v1"
	"open-cluster-management.io/iam-policy-controller/controllers"
	common "open-cluster-management.io/iam-policy-controller/pkg/common"
	"open-cluster-management.io/iam-policy-controller/version"
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
	zflags := zaputil.FlagConfig{
		LevelName:   "log-level",
		EncoderName: "log-encoder",
	}

	zflags.Bind(flag.CommandLine)
	klog.InitFlags(flag.CommandLine)

	// Add flags registered by imported packages (e.g. glog and controller-runtime)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	var clusterName, eventOnParent, hubConfigPath, targetKubeConfig, metricsAddr, probeAddr string
	var frequency uint
	var enableLease, enableLeaderElection bool

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
		&hubConfigPath,
		"hub-kubeconfig-path",
		"/var/run/klusterlet/kubeconfig",
		"Path to the hub kubeconfig")
	pflag.StringVar(
		&targetKubeConfig,
		"target-kubeconfig-path",
		"",
		"A path to an alternative kubeconfig for policy evaluation and enforcement.",
	)
	pflag.BoolVar(&enableLeaderElection, "leader-elect", true,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	pflag.StringVar(&metricsAddr, "metrics-bind-address", ":8383", "The address the metric endpoint binds to.")
	pflag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")

	pflag.Parse()

	ctrlZap, err := zflags.BuildForCtrl()
	if err != nil {
		panic(fmt.Sprintf("Failed to build zap logger for controller: %v", err))
	}

	ctrl.SetLogger(zapr.NewLogger(ctrlZap))

	klogZap, err := zaputil.BuildForKlog(zflags.GetConfig(), flag.CommandLine)
	if err != nil {
		setupLog.Error(err, "Failed to build zap logger for klog, those logs will not go through zap")
	} else {
		klog.SetLogger(zapr.NewLogger(klogZap).WithName("klog"))
	}

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

	// Create a new manager to provide shared dependencies and start components
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	var targetK8sConfig *rest.Config
	var targetK8sClient kubernetes.Interface
	var targetK8sDynamicClient dynamic.Interface

	if targetKubeConfig == "" {
		targetK8sConfig = mgr.GetConfig()
	} else {
		var err error

		targetK8sConfig, err = clientcmd.BuildConfigFromFlags("", targetKubeConfig)
		if err != nil {
			setupLog.Error(err, "Failed to load the target kubeconfig", "path", targetKubeConfig)
			os.Exit(1)
		}

		setupLog.Info(
			"Overrode the target Kubernetes cluster for policy evaluation and enforcement", "path", targetKubeConfig,
		)
	}

	targetK8sClient = kubernetes.NewForConfigOrDie(targetK8sConfig)
	targetK8sDynamicClient = dynamic.NewForConfigOrDie(targetK8sConfig)

	controllers.Initialize(&targetK8sClient, &targetK8sDynamicClient, eventOnParent)

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
			setupLog.V(2).Info("Got operator namespace", "Namespace", operatorNs)
			setupLog.Info("Starting lease controller to report status")
			// Always use the cluster that is running the controller for the lease.
			leaseUpdater := lease.NewLeaseUpdater(
				kubernetes.NewForConfigOrDie(mgr.GetConfig()),
				"iam-policy-controller",
				operatorNs,
			)

			hubCfg, err := clientcmd.BuildConfigFromFlags("", hubConfigPath)
			if err != nil {
				setupLog.Error(err, "Could not load hub config, lease updater not set with config")
			} else {
				leaseUpdater = leaseUpdater.WithHubLeaseConfig(hubCfg, clusterName)
			}

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

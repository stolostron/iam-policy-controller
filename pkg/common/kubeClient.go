// Licensed Materials - Property of IBM
// (c) Copyright IBM Corporation 2018. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package common

import (
	"context"
	base64 "encoding/base64"
	"regexp"

	"github.com/golang/glog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	// KubeClient is a k8s client used for k8s native resources
	KubeClient *kubernetes.Interface
	// KubeConfig is the given kubeconfig at startup
	KubeConfig *rest.Config

	HubConfig *rest.Config
)

// Initialize to initialize some controller variables
func Initialize(kClient *kubernetes.Interface, cfg *rest.Config) {
	KubeClient = kClient
	KubeConfig = cfg
}

func LoadHubConfig(namespace string, secretname string) (*rest.Config, error) {
	if HubConfig == nil {
		secretsClient := (*KubeClient).CoreV1().Secrets(namespace)

		hubSecret, err := secretsClient.Get(context.TODO(), secretname, metav1.GetOptions{})
		if err != nil {
			glog.Errorf("Error Getting HubConfig Secret:  %v", err)

			return nil, err
		}

		secretkconfig := string(hubSecret.Data["kubeconfig"])
		crt := base64.StdEncoding.EncodeToString(hubSecret.Data["tls.crt"])
		key := base64.StdEncoding.EncodeToString(hubSecret.Data["tls.key"])

		re := regexp.MustCompile(`(client-certificate:\s+tls.crt)`)
		secretkconfig = re.ReplaceAllString(secretkconfig, "client-certificate-data: "+crt)

		re = regexp.MustCompile(`(client-key:\s+tls.key)`)
		secretkconfig = re.ReplaceAllString(secretkconfig, "client-key-data: "+key)

		HubConfig, err = clientcmd.RESTConfigFromKubeConfig([]byte(secretkconfig))
		if err != nil {
			glog.Errorf("Error getting Rest config for Hub:  %v", err)

			return nil, err
		}
	}

	return HubConfig, nil
}

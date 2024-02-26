// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package e2e

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/ghodss/yaml"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

var (
	testNamespace         string
	defaultTimeoutSeconds int
	kubeconfigManaged     string
	clientManaged         kubernetes.Interface
	clientManagedDynamic  dynamic.Interface
	gvrIamPolicy          schema.GroupVersionResource
	gvrPolicy             schema.GroupVersionResource
)

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Iam Policy Controller e2e Suite")
}

func init() {
	klog.SetOutput(GinkgoWriter)
	klog.InitFlags(nil)
	flag.StringVar(&kubeconfigManaged, "kubeconfig_managed", "../../kubeconfig_managed_e2e",
		"Location of the kubeconfig to use; defaults to current kubeconfig if set to an empty string")
}

var _ = BeforeSuite(func() {
	By("Setup Hub client")
	gvrIamPolicy = schema.GroupVersionResource{
		Group:    "policy.open-cluster-management.io",
		Version:  "v1",
		Resource: "iampolicies",
	}
	gvrPolicy = schema.GroupVersionResource{
		Group:    "policy.open-cluster-management.io",
		Version:  "v1",
		Resource: "policies",
	}

	clientManaged = NewKubeClient("", kubeconfigManaged, "")
	clientManagedDynamic = NewKubeClientDynamic("", kubeconfigManaged, "")
	testNamespace = "managed"
	testNamespaces := []string{testNamespace, "range1", "range2"}
	defaultTimeoutSeconds = 60
	By("Create Namespaces if needed")
	namespaces := clientManaged.CoreV1().Namespaces()
	for _, ns := range testNamespaces {
		if _, err := namespaces.Get(context.TODO(), ns, metav1.GetOptions{}); err != nil && errors.IsNotFound(err) {
			Expect(namespaces.Create(context.TODO(), &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: ns,
				},
			}, metav1.CreateOptions{})).NotTo(BeNil())
		}
		Expect(namespaces.Get(context.TODO(), ns, metav1.GetOptions{})).NotTo(BeNil())
	}
})

func NewKubeClient(url, kubeconfig, context string) kubernetes.Interface {
	klog.V(5).Infof("Create kubeclient for url %s using kubeconfig path %s\n", url, kubeconfig)

	config, err := LoadConfig(url, kubeconfig, context)
	if err != nil {
		panic(err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	return clientset
}

func NewKubeClientDynamic(url, kubeconfig, context string) dynamic.Interface {
	klog.V(5).Infof("Create kubeclient dynamic for url %s using kubeconfig path %s\n", url, kubeconfig)

	config, err := LoadConfig(url, kubeconfig, context)
	if err != nil {
		panic(err)
	}

	clientset, err := dynamic.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	return clientset
}

func LoadConfig(url, kubeconfig, context string) (*rest.Config, error) {
	if kubeconfig == "" {
		kubeconfig = os.Getenv("KUBECONFIG")
	}

	klog.V(5).Infof("Kubeconfig path %s\n", kubeconfig)

	// If we have an explicit indication of where the kubernetes config lives, read that.
	if kubeconfig != "" {
		if context == "" {
			return clientcmd.BuildConfigFromFlags(url, kubeconfig)
		}

		return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig},
			&clientcmd.ConfigOverrides{
				CurrentContext: context,
			}).ClientConfig()
	}

	// If not, try the in-cluster config.
	if c, err := rest.InClusterConfig(); err == nil {
		return c, nil
	}

	// If no in-cluster config, try the default location in the user's home directory.
	if usr, err := user.Current(); err == nil {
		klog.V(5).Infof("clientcmd.BuildConfigFromFlags for url %s using %s\n", url,
			filepath.Join(usr.HomeDir, ".kube", "config"))

		if c, err := clientcmd.BuildConfigFromFlags("", filepath.Join(usr.HomeDir, ".kube", "config")); err == nil {
			return c, nil
		}
	}

	return nil, fmt.Errorf("could not create a valid kubeconfig")
}

// Kubectl executes kubectl commands
func Kubectl(args ...string) {
	cmd := exec.Command("kubectl", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// in case of failure, print command output (including error)
		//nolint:forbidigo
		fmt.Printf("%s\n", output)
		Fail(fmt.Sprintf("Error: %v", err))
	}
}

// GetWithTimeout keeps polling to get the object for timeout seconds until wantFound is met
// (true for found, false for not found)
func GetWithTimeout(
	gvr schema.GroupVersionResource,
	name, namespace string,
	wantFound bool,
) *unstructured.Unstructured {
	var obj *unstructured.Unstructured

	Eventually(func() error {
		var err error
		namespace := clientManagedDynamic.Resource(gvr).Namespace(namespace)
		obj, err = namespace.Get(context.TODO(), name, metav1.GetOptions{})
		if wantFound && err != nil {
			return err
		}
		if !wantFound && err == nil {
			return fmt.Errorf("expected to return IsNotFound error")
		}
		if !wantFound && err != nil && !errors.IsNotFound(err) {
			return err
		}

		return nil
	}, defaultTimeoutSeconds, 1).Should(BeNil())

	if wantFound {
		return obj
	}

	return nil
}

func CreateIAMPolicyWithParent(parentPolicyYAML, parentPolicyName, iamPolicyYAML string) {
	By("Creating the parent policy")
	Kubectl("apply", "-f", parentPolicyYAML, "-n", testNamespace, "--kubeconfig", kubeconfigManaged)
	parent := GetWithTimeout(gvrPolicy, parentPolicyName, testNamespace, true)
	Expect(parent).NotTo(BeNil())

	plcDef := ParseYaml(iamPolicyYAML)
	ownerRefs := plcDef.GetOwnerReferences()
	ownerRefs[0].UID = parent.GetUID()
	plcDef.SetOwnerReferences(ownerRefs)

	By("Creating the iam policy with the owner reference")

	_, err := clientManagedDynamic.Resource(gvrIamPolicy).Namespace(testNamespace).
		Create(context.TODO(), plcDef, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())

	By("Verifying the iam policy exists")

	plc := GetWithTimeout(gvrIamPolicy, plcDef.GetName(), testNamespace, true)
	Expect(plc).NotTo(BeNil())
}

// ParseYaml read given yaml file and unmarshal it to &unstructured.Unstructured{}
func ParseYaml(file string) *unstructured.Unstructured {
	yamlFile, err := os.ReadFile(file)
	Expect(err).ToNot(HaveOccurred())

	yamlPlc := &unstructured.Unstructured{}
	err = yaml.Unmarshal(yamlFile, yamlPlc)
	Expect(err).ToNot(HaveOccurred())

	return yamlPlc
}

func GetMatchingEvents(
	namespace, objName, reasonRegex, msgRegex string,
) []corev1.Event {
	var eventList *corev1.EventList

	EventuallyWithOffset(1, func() error {
		var err error
		eventList, err = clientManaged.CoreV1().Events(namespace).List(context.TODO(), metav1.ListOptions{})

		return err
	}, defaultTimeoutSeconds, 1).ShouldNot(HaveOccurred())

	matchingEvents := make([]corev1.Event, 0)
	msgMatcher := regexp.MustCompile(msgRegex)
	reasonMatcher := regexp.MustCompile(reasonRegex)

	for _, event := range eventList.Items {
		if event.InvolvedObject.Name == objName && reasonMatcher.MatchString(event.Reason) &&
			msgMatcher.MatchString(event.Message) {
			matchingEvents = append(matchingEvents, event)
		}
	}

	return matchingEvents
}

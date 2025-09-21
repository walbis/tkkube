package mocks

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/discovery"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

// MockKubernetesClients provides mock Kubernetes clients for testing
type MockKubernetesClients struct {
	KubeClient      kubernetes.Interface
	DynamicClient   dynamic.Interface
	DiscoveryClient discovery.DiscoveryInterface
}

// NewMockKubernetesClients creates mock Kubernetes clients with test data
func NewMockKubernetesClients() *MockKubernetesClients {
	// Create fake Kubernetes client with test namespaces
	kubeClient := fake.NewSimpleClientset(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "default",
			},
		},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "kube-system",
			},
		},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-namespace",
			},
		},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "openshift-config",
			},
		},
	)

	// Create fake dynamic client
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme)

	// Create fake discovery client with API resources
	discoveryClient := &fakediscovery.FakeDiscovery{
		Fake: &kubeClient.Fake,
	}

	// Add some API resource groups
	discoveryClient.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{
					Name:         "pods",
					SingularName: "pod",
					Namespaced:   true,
					Kind:         "Pod",
					Verbs:        []string{"get", "list", "create", "update", "patch", "watch", "delete"},
				},
				{
					Name:         "services",
					SingularName: "service",
					Namespaced:   true,
					Kind:         "Service",
					Verbs:        []string{"get", "list", "create", "update", "patch", "watch", "delete"},
				},
				{
					Name:         "configmaps",
					SingularName: "configmap",
					Namespaced:   true,
					Kind:         "ConfigMap",
					Verbs:        []string{"get", "list", "create", "update", "patch", "watch", "delete"},
				},
				{
					Name:         "secrets",
					SingularName: "secret",
					Namespaced:   true,
					Kind:         "Secret",
					Verbs:        []string{"get", "list", "create", "update", "patch", "watch", "delete"},
				},
			},
		},
		{
			GroupVersion: "apps/v1",
			APIResources: []metav1.APIResource{
				{
					Name:         "deployments",
					SingularName: "deployment",
					Namespaced:   true,
					Kind:         "Deployment",
					Verbs:        []string{"get", "list", "create", "update", "patch", "watch", "delete"},
				},
				{
					Name:         "replicasets",
					SingularName: "replicaset",
					Namespaced:   true,
					Kind:         "ReplicaSet",
					Verbs:        []string{"get", "list", "create", "update", "patch", "watch", "delete"},
				},
			},
		},
	}

	return &MockKubernetesClients{
		KubeClient:      kubeClient,
		DynamicClient:   dynamicClient,
		DiscoveryClient: discoveryClient,
	}
}

// MockDiscoveryClient provides additional methods for testing
type MockDiscoveryClient struct {
	*fakediscovery.FakeDiscovery
	shouldError bool
}

// NewMockDiscoveryClient creates a discovery client that can simulate errors
func NewMockDiscoveryClient(shouldError bool) *MockDiscoveryClient {
	fake := &fakediscovery.FakeDiscovery{}
	return &MockDiscoveryClient{
		FakeDiscovery: fake,
		shouldError:   shouldError,
	}
}

// ServerPreferredNamespacedResources returns API resources or error based on configuration
func (m *MockDiscoveryClient) ServerPreferredNamespacedResources() ([]*metav1.APIResourceList, error) {
	if m.shouldError {
		return nil, &discovery.ErrGroupDiscoveryFailed{
			Groups: map[schema.GroupVersion]error{
				{Group: "test", Version: "v1"}: context.DeadlineExceeded,
			},
		}
	}
	return m.FakeDiscovery.ServerPreferredNamespacedResources()
}

// ServerVersion returns a mock server version
func (m *MockDiscoveryClient) ServerVersion() (*version.Info, error) {
	if m.shouldError {
		return nil, context.DeadlineExceeded
	}
	return &version.Info{
		Major:      "1",
		Minor:      "25",
		GitVersion: "v1.25.0",
	}, nil
}
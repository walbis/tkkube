package restore

import (
	"context"
	"fmt"
	"strings"
	"time"

	sharedconfig "shared-config/config"
	
	"k8s.io/client-go/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

// RestoreValidator validates restore operations before execution
type RestoreValidator struct {
	config        *sharedconfig.SharedConfig
	k8sClient     kubernetes.Interface
	discoveryClient discovery.DiscoveryInterface
}

// ValidationReport contains the results of restore validation
type ValidationReport struct {
	Valid             bool                 `json:"valid"`
	Errors            []ValidationError    `json:"errors"`
	Warnings          []ValidationWarning  `json:"warnings"`
	ResourcesSummary  ResourcesSummary     `json:"resources_summary"`
	ClusterInfo       ClusterInfo          `json:"cluster_info"`
	CompatibilityCheck CompatibilityCheck  `json:"compatibility_check"`
	Timestamp         time.Time            `json:"timestamp"`
}

// ValidationSummary provides summary of validation results
type ValidationSummary struct {
	TotalChecks      int `json:"total_checks"`
	PassedChecks     int `json:"passed_checks"`
	FailedChecks     int `json:"failed_checks"`
	WarningChecks    int `json:"warning_checks"`
	ValidationScore  float64 `json:"validation_score"`
}

// ValidationError represents a validation error that would prevent restore
type ValidationError struct {
	Type        string                 `json:"type"`
	Message     string                 `json:"message"`
	Resource    string                 `json:"resource,omitempty"`
	Namespace   string                 `json:"namespace,omitempty"`
	Severity    string                 `json:"severity"`
	Suggestions []string               `json:"suggestions,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// ValidationWarning represents a validation warning that won't prevent restore
type ValidationWarning struct {
	Type        string                 `json:"type"`
	Message     string                 `json:"message"`
	Resource    string                 `json:"resource,omitempty"`
	Namespace   string                 `json:"namespace,omitempty"`
	Impact      string                 `json:"impact"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// ResourcesSummary provides summary of resources to be restored
type ResourcesSummary struct {
	TotalResources      int                    `json:"total_resources"`
	ResourcesByType     map[string]int         `json:"resources_by_type"`
	ResourcesByNamespace map[string]int        `json:"resources_by_namespace"`
	NamespaceScoped     int                    `json:"namespace_scoped"`
	ClusterScoped       int                    `json:"cluster_scoped"`
	CustomResources     int                    `json:"custom_resources"`
	EstimatedSize       int64                  `json:"estimated_size_bytes"`
}

// ClusterInfo contains information about the target cluster
type ClusterInfo struct {
	Version            string            `json:"version"`
	ServerVersion      string            `json:"server_version"`
	Platform           string            `json:"platform"`
	NodeCount          int               `json:"node_count"`
	NamespaceCount     int               `json:"namespace_count"`
	AvailableResources []string          `json:"available_resources"`
	Features           map[string]bool   `json:"features"`
}

// CompatibilityCheck validates compatibility between backup and target cluster
type CompatibilityCheck struct {
	Compatible           bool              `json:"compatible"`
	KubernetesVersion    VersionCheck      `json:"kubernetes_version"`
	APIVersions          []APIVersionCheck `json:"api_versions"`
	Features             []FeatureCheck    `json:"features"`
	StorageClasses       []StorageCheck    `json:"storage_classes"`
	CustomResourceDefs   []CRDCheck        `json:"custom_resource_definitions"`
}

// VersionCheck validates Kubernetes version compatibility
type VersionCheck struct {
	BackupVersion   string `json:"backup_version"`
	ClusterVersion  string `json:"cluster_version"`
	Compatible      bool   `json:"compatible"`
	Message         string `json:"message,omitempty"`
}

// APIVersionCheck validates API version compatibility
type APIVersionCheck struct {
	GroupVersion    string `json:"group_version"`
	Kind            string `json:"kind"`
	Available       bool   `json:"available"`
	Deprecated      bool   `json:"deprecated"`
	RemovalVersion  string `json:"removal_version,omitempty"`
	Migration       string `json:"migration,omitempty"`
}

// FeatureCheck validates feature compatibility
type FeatureCheck struct {
	Feature     string `json:"feature"`
	Required    bool   `json:"required"`
	Available   bool   `json:"available"`
	Alternative string `json:"alternative,omitempty"`
}

// StorageCheck validates storage class compatibility
type StorageCheck struct {
	StorageClass    string `json:"storage_class"`
	Available       bool   `json:"available"`
	Provisioner     string `json:"provisioner"`
	Compatible      bool   `json:"compatible"`
	Alternative     string `json:"alternative,omitempty"`
}

// CRDCheck validates Custom Resource Definition compatibility
type CRDCheck struct {
	CRDName         string `json:"crd_name"`
	Group           string `json:"group"`
	Version         string `json:"version"`
	Available       bool   `json:"available"`
	VersionMatch    bool   `json:"version_match"`
	SchemaValid     bool   `json:"schema_valid"`
}

// NewRestoreValidator creates a new restore validator
func NewRestoreValidator(config *sharedconfig.SharedConfig, k8sClient kubernetes.Interface) *RestoreValidator {
	discoveryClient := k8sClient.Discovery()
	
	return &RestoreValidator{
		config:          config,
		k8sClient:       k8sClient,
		discoveryClient: discoveryClient,
	}
}

// ValidateRestore performs comprehensive validation of a restore request
func (rv *RestoreValidator) ValidateRestore(ctx context.Context, request RestoreRequest) (*ValidationReport, error) {
	report := &ValidationReport{
		Valid:     true,
		Errors:    make([]ValidationError, 0),
		Warnings:  make([]ValidationWarning, 0),
		Timestamp: time.Now(),
	}

	// Validate cluster accessibility
	if err := rv.validateClusterAccess(ctx, report); err != nil {
		return nil, fmt.Errorf("cluster access validation failed: %v", err)
	}

	// Gather cluster information
	if err := rv.gatherClusterInfo(ctx, report); err != nil {
		return nil, fmt.Errorf("failed to gather cluster info: %v", err)
	}

	// Validate backup metadata
	if err := rv.validateBackupMetadata(ctx, request, report); err != nil {
		rv.addError(report, "backup_metadata", err.Error(), "", "", "critical", nil)
	}

	// Validate target namespaces
	rv.validateTargetNamespaces(ctx, request, report)

	// Validate permissions
	rv.validatePermissions(ctx, request, report)

	// Validate resource compatibility
	rv.validateResourceCompatibility(ctx, request, report)

	// Validate storage requirements
	rv.validateStorageRequirements(ctx, request, report)

	// Calculate validation score
	report.ResourcesSummary.ValidationScore = rv.calculateValidationScore(report)

	// Set overall validity
	report.Valid = len(report.Errors) == 0

	return report, nil
}

// validateClusterAccess verifies that the cluster is accessible
func (rv *RestoreValidator) validateClusterAccess(ctx context.Context, report *ValidationReport) error {
	// Test basic cluster connectivity
	_, err := rv.k8sClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		rv.addError(report, "cluster_access", "Cannot access Kubernetes cluster", "", "", "critical", 
			[]string{"Check kubeconfig", "Verify cluster connectivity", "Check authentication"})
		return err
	}

	return nil
}

// gatherClusterInfo collects information about the target cluster
func (rv *RestoreValidator) gatherClusterInfo(ctx context.Context, report *ValidationReport) error {
	// Get server version
	version, err := rv.k8sClient.Discovery().ServerVersion()
	if err != nil {
		rv.addWarning(report, "cluster_info", "Failed to get server version", "", "", "medium", nil)
	} else {
		report.ClusterInfo.Version = version.GitVersion
		report.ClusterInfo.ServerVersion = version.String()
	}

	// Count nodes
	nodes, err := rv.k8sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		rv.addWarning(report, "cluster_info", "Failed to get node count", "", "", "low", nil)
	} else {
		report.ClusterInfo.NodeCount = len(nodes.Items)
	}

	// Count namespaces
	namespaces, err := rv.k8sClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		rv.addWarning(report, "cluster_info", "Failed to get namespace count", "", "", "low", nil)
	} else {
		report.ClusterInfo.NamespaceCount = len(namespaces.Items)
	}

	// Get available API resources
	resources, err := rv.discoveryClient.ServerPreferredResources()
	if err != nil {
		rv.addWarning(report, "cluster_info", "Failed to get available resources", "", "", "medium", nil)
	} else {
		availableResources := make([]string, 0)
		for _, resourceList := range resources {
			for _, resource := range resourceList.APIResources {
				gvr := fmt.Sprintf("%s/%s", resourceList.GroupVersion, resource.Name)
				availableResources = append(availableResources, gvr)
			}
		}
		report.ClusterInfo.AvailableResources = availableResources
	}

	// Detect platform
	rv.detectPlatform(ctx, report)

	return nil
}

// detectPlatform attempts to detect the Kubernetes platform
func (rv *RestoreValidator) detectPlatform(ctx context.Context, report *ValidationReport) {
	platform := "unknown"

	// Check for OpenShift
	_, err := rv.k8sClient.RESTClient().Get().AbsPath("/apis/config.openshift.io/v1").DoRaw(ctx)
	if err == nil {
		platform = "openshift"
	} else {
		// Check for common cloud providers
		nodes, err := rv.k8sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{Limit: 1})
		if err == nil && len(nodes.Items) > 0 {
			node := nodes.Items[0]
			
			if strings.Contains(node.Spec.ProviderID, "aws") {
				platform = "eks"
			} else if strings.Contains(node.Spec.ProviderID, "gce") {
				platform = "gke"
			} else if strings.Contains(node.Spec.ProviderID, "azure") {
				platform = "aks"
			}
		}
	}

	report.ClusterInfo.Platform = platform
}

// validateBackupMetadata validates the backup metadata and accessibility
func (rv *RestoreValidator) validateBackupMetadata(ctx context.Context, request RestoreRequest, report *ValidationReport) error {
	// This would validate backup exists in MinIO and metadata is accessible
	// For now, just basic validation
	
	if request.BackupID == "" {
		return fmt.Errorf("backup ID is required")
	}

	if request.ClusterName == "" {
		return fmt.Errorf("cluster name is required")
	}

	// Validate backup exists (placeholder)
	// In real implementation:
	// 1. Connect to MinIO
	// 2. Check backup exists
	// 3. Load backup metadata
	// 4. Validate backup integrity

	return nil
}

// validateTargetNamespaces validates the target namespaces for restore
func (rv *RestoreValidator) validateTargetNamespaces(ctx context.Context, request RestoreRequest, report *ValidationReport) {
	if len(request.TargetNamespaces) == 0 {
		rv.addWarning(report, "namespaces", "No target namespaces specified, will restore to original namespaces", "", "", "low", nil)
		return
	}

	for _, namespace := range request.TargetNamespaces {
		// Check if namespace exists
		_, err := rv.k8sClient.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
		if err != nil {
			rv.addWarning(report, "namespaces", fmt.Sprintf("Target namespace '%s' does not exist, will be created", namespace), "", namespace, "medium", 
				[]string{"Create namespace manually", "Ensure proper RBAC permissions"})
		}

		// Validate namespace name
		if !rv.isValidNamespaceName(namespace) {
			rv.addError(report, "namespaces", fmt.Sprintf("Invalid namespace name: '%s'", namespace), "", namespace, "medium", 
				[]string{"Use valid DNS label format", "Check Kubernetes naming conventions"})
		}
	}
}

// validatePermissions validates that the service account has necessary permissions
func (rv *RestoreValidator) validatePermissions(ctx context.Context, request RestoreRequest, report *ValidationReport) {
	// Check basic permissions
	requiredPermissions := []PermissionCheck{
		{Resource: "namespaces", Verbs: []string{"get", "list", "create"}},
		{Resource: "pods", Verbs: []string{"get", "list", "create", "update", "patch"}},
		{Resource: "services", Verbs: []string{"get", "list", "create", "update", "patch"}},
		{Resource: "configmaps", Verbs: []string{"get", "list", "create", "update", "patch"}},
		{Resource: "secrets", Verbs: []string{"get", "list", "create", "update", "patch"}},
	}

	for _, perm := range requiredPermissions {
		for _, verb := range perm.Verbs {
			// Use SelfSubjectAccessReview to check permissions
			canAccess, err := rv.checkPermission(ctx, perm.Resource, verb, "")
			if err != nil {
				rv.addWarning(report, "permissions", fmt.Sprintf("Failed to check permission for %s:%s", verb, perm.Resource), "", "", "medium", nil)
			} else if !canAccess {
				rv.addError(report, "permissions", fmt.Sprintf("Missing permission: %s on %s", verb, perm.Resource), "", "", "high", 
					[]string{"Grant necessary RBAC permissions", "Check service account roles"})
			}
		}
	}
}

// validateResourceCompatibility validates that backup resources are compatible with target cluster
func (rv *RestoreValidator) validateResourceCompatibility(ctx context.Context, request RestoreRequest, report *ValidationReport) {
	// This would analyze backup resources and check compatibility
	// For now, simulate basic checks
	
	compatCheck := CompatibilityCheck{
		Compatible: true,
		APIVersions: make([]APIVersionCheck, 0),
		Features: make([]FeatureCheck, 0),
	}

	// Check Kubernetes version compatibility
	if report.ClusterInfo.Version != "" {
		compatCheck.KubernetesVersion = VersionCheck{
			BackupVersion:  "1.28.0", // Would come from backup metadata
			ClusterVersion: report.ClusterInfo.Version,
			Compatible:     true, // Would be calculated
		}
	}

	// Check common deprecated APIs
	deprecatedAPIs := []APIVersionCheck{
		{
			GroupVersion:   "extensions/v1beta1",
			Kind:           "Ingress",
			Available:      false,
			Deprecated:     true,
			RemovalVersion: "1.22",
			Migration:      "Use networking.k8s.io/v1 Ingress",
		},
	}

	for _, api := range deprecatedAPIs {
		if rv.isAPIAvailable(ctx, api.GroupVersion, api.Kind) {
			api.Available = true
		}
		compatCheck.APIVersions = append(compatCheck.APIVersions, api)
		
		if !api.Available && api.Deprecated {
			rv.addError(report, "compatibility", fmt.Sprintf("API %s for %s is not available", api.GroupVersion, api.Kind), api.Kind, "", "high", 
				[]string{api.Migration})
		}
	}

	report.CompatibilityCheck = compatCheck
}

// validateStorageRequirements validates storage requirements for restore
func (rv *RestoreValidator) validateStorageRequirements(ctx context.Context, request RestoreRequest, report *ValidationReport) {
	// Check storage classes
	storageClasses, err := rv.k8sClient.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		rv.addWarning(report, "storage", "Failed to list storage classes", "", "", "medium", nil)
		return
	}

	// Basic storage validation
	if len(storageClasses.Items) == 0 {
		rv.addWarning(report, "storage", "No storage classes found", "", "", "medium", 
			[]string{"Ensure storage classes are available", "Check storage provisioner"})
	}

	// Check for default storage class
	hasDefault := false
	for _, sc := range storageClasses.Items {
		if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			hasDefault = true
			break
		}
	}

	if !hasDefault {
		rv.addWarning(report, "storage", "No default storage class found", "", "", "medium", 
			[]string{"Set a default storage class", "Specify storage class in PVC templates"})
	}
}

// Helper methods

func (rv *RestoreValidator) addError(report *ValidationReport, errorType, message, resource, namespace, severity string, suggestions []string) {
	report.Errors = append(report.Errors, ValidationError{
		Type:        errorType,
		Message:     message,
		Resource:    resource,
		Namespace:   namespace,
		Severity:    severity,
		Suggestions: suggestions,
	})
}

func (rv *RestoreValidator) addWarning(report *ValidationReport, warningType, message, resource, namespace, impact string, metadata map[string]interface{}) {
	report.Warnings = append(report.Warnings, ValidationWarning{
		Type:      warningType,
		Message:   message,
		Resource:  resource,
		Namespace: namespace,
		Impact:    impact,
		Metadata:  metadata,
	})
}

func (rv *RestoreValidator) isValidNamespaceName(name string) bool {
	// Basic DNS label validation
	if len(name) == 0 || len(name) > 63 {
		return false
	}
	
	for _, char := range name {
		if !((char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-') {
			return false
		}
	}
	
	return !strings.HasPrefix(name, "-") && !strings.HasSuffix(name, "-")
}

func (rv *RestoreValidator) checkPermission(ctx context.Context, resource, verb, namespace string) (bool, error) {
	// This would use SelfSubjectAccessReview to check permissions
	// Simplified implementation
	return true, nil
}

func (rv *RestoreValidator) isAPIAvailable(ctx context.Context, groupVersion, kind string) bool {
	// Check if API version is available in cluster
	gv, err := schema.ParseGroupVersion(groupVersion)
	if err != nil {
		return false
	}

	resources, err := rv.discoveryClient.ServerResourcesForGroupVersion(groupVersion)
	if err != nil {
		return false
	}

	for _, resource := range resources.APIResources {
		if resource.Kind == kind {
			return true
		}
	}

	return false
}

func (rv *RestoreValidator) calculateValidationScore(report *ValidationReport) float64 {
	if len(report.Errors) == 0 && len(report.Warnings) == 0 {
		return 100.0
	}

	totalChecks := len(report.Errors) + len(report.Warnings)
	if totalChecks == 0 {
		return 100.0
	}

	// Weight errors more heavily than warnings
	errorWeight := 1.0
	warningWeight := 0.3

	totalWeight := float64(len(report.Errors))*errorWeight + float64(len(report.Warnings))*warningWeight
	maxWeight := float64(totalChecks) * errorWeight

	score := (maxWeight - totalWeight) / maxWeight * 100
	if score < 0 {
		score = 0
	}

	return score
}

// PermissionCheck represents a permission validation check
type PermissionCheck struct {
	Resource string   `json:"resource"`
	Verbs    []string `json:"verbs"`
}
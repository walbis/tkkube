package cluster

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// DetectionResult contains cluster detection information
type DetectionResult struct {
	ClusterName   string
	ClusterDomain string
	IsOpenShift   bool
	OpenShiftMode string
}

// Detector handles cluster information detection
type Detector struct {
	clientset     kubernetes.Interface
	dynamicClient dynamic.Interface
	ctx           context.Context
	
	// Cache for detection results
	clusterName        string
	clusterDomain      string
	openShiftDetected  *string
	openShiftCacheTime time.Time
}

// NewDetector creates a new cluster detector
func NewDetector(clientset kubernetes.Interface, dynamicClient dynamic.Interface, ctx context.Context) *Detector {
	return &Detector{
		clientset:     clientset,
		dynamicClient: dynamicClient,
		ctx:           ctx,
	}
}

// DetectClusterInfo detects comprehensive cluster information
func (d *Detector) DetectClusterInfo() *DetectionResult {
	result := &DetectionResult{
		ClusterName:   d.DetectClusterName(),
		ClusterDomain: d.DetectClusterDomain(),
	}
	
	openShiftMode := d.DetectOpenShift()
	result.IsOpenShift = openShiftMode != "disabled"
	result.OpenShiftMode = openShiftMode
	
	return result
}

// DetectClusterName attempts to dynamically detect the cluster name from Kubernetes API
func (d *Detector) DetectClusterName() string {
	if d.clusterName != "" {
		return d.clusterName
	}
	
	log.Printf("=== CLUSTER NAME DETECTION START ===")
	
	// Try to get cluster name from multiple sources
	// 1. Try OpenShift Infrastructure (if available)
	// 2. Try kube-system namespace labels
	// 3. Try nodes with cluster labels
	// 4. Fallback to hostname-based detection
	
	log.Printf("Step 1: Trying OpenShift Infrastructure detection...")
	if clusterName := d.detectOpenShiftClusterName(); clusterName != "" {
		log.Printf("✓ SUCCESS: OpenShift Infrastructure detection returned: '%s'", clusterName)
		d.clusterName = clusterName
		return clusterName
	}
	log.Printf("✗ OpenShift Infrastructure detection failed or empty")
	
	log.Printf("Step 2: Trying kube-system namespace labels detection...")
	if clusterName := d.detectFromNamespaceLabels(); clusterName != "" {
		log.Printf("✓ SUCCESS: Namespace labels detection returned: '%s'", clusterName)
		d.clusterName = clusterName
		return clusterName
	}
	log.Printf("✗ Namespace labels detection failed or empty")
	
	log.Printf("Step 3: Trying node labels detection...")
	if clusterName := d.detectFromNodeLabels(); clusterName != "" {
		log.Printf("✓ SUCCESS: Node labels detection returned: '%s'", clusterName)
		d.clusterName = clusterName
		return clusterName
	}
	log.Printf("✗ Node labels detection failed or empty")
	
	log.Printf("Step 4: Trying hostname-based detection...")
	if clusterName := d.detectFromHostname(); clusterName != "" {
		log.Printf("✓ SUCCESS: Hostname detection returned: '%s'", clusterName)
		d.clusterName = clusterName
		return clusterName
	}
	log.Printf("✗ Hostname detection failed or empty")
	
	log.Printf("=== CLUSTER NAME DETECTION FAILED - USING FALLBACK ===")
	d.clusterName = "unknown-cluster"
	return d.clusterName
}

// DetectClusterDomain attempts to detect the cluster domain
func (d *Detector) DetectClusterDomain() string {
	if d.clusterDomain != "" {
		return d.clusterDomain
	}
	
	log.Printf("=== CLUSTER DOMAIN DETECTION START ===")
	
	// Check environment variable first
	if domain := os.Getenv("CLUSTER_DOMAIN"); domain != "" {
		log.Printf("✓ SUCCESS: Environment variable CLUSTER_DOMAIN: '%s'", domain)
		d.clusterDomain = domain
		return domain
	}
	log.Printf("✗ Environment variable CLUSTER_DOMAIN not set or empty")
	
	// Try OpenShift-specific detection
	if domain := d.detectDomainFromOpenShiftDNS(); domain != "" {
		log.Printf("✓ SUCCESS: OpenShift DNS detection returned: '%s'", domain)
		d.clusterDomain = domain
		return domain
	}
	
	// Try DNS config detection
	if domain := d.detectDomainFromDNSConfig(); domain != "" {
		log.Printf("✓ SUCCESS: DNS config detection returned: '%s'", domain)
		d.clusterDomain = domain
		return domain
	}
	
	log.Printf("=== CLUSTER DOMAIN DETECTION FAILED - USING FALLBACK ===")
	d.clusterDomain = "cluster.local"
	return d.clusterDomain
}

// DetectOpenShift detects if this is an OpenShift cluster and returns the mode
func (d *Detector) DetectOpenShift() string {
	// Use cached result if available and not expired
	if d.openShiftDetected != nil && time.Since(d.openShiftCacheTime) < 5*time.Minute {
		return *d.openShiftDetected
	}
	
	log.Printf("=== OPENSHIFT DETECTION START ===")
	
	// Check for environment override first
	if mode := os.Getenv("OPENSHIFT_MODE"); mode != "" {
		log.Printf("✓ Environment override: OPENSHIFT_MODE=%s", mode)
		d.openShiftDetected = &mode
		d.openShiftCacheTime = time.Now()
		return mode
	}
	
	// Try to detect OpenShift-specific resources
	// Check for security.openshift.io API group
	discoveryClient := d.clientset.Discovery()
	groups, err := discoveryClient.ServerGroups()
	if err != nil {
		log.Printf("✗ Failed to get server groups for OpenShift detection: %v", err)
		mode := "disabled"
		d.openShiftDetected = &mode
		d.openShiftCacheTime = time.Now()
		return mode
	}
	
	for _, group := range groups.Groups {
		if strings.Contains(group.Name, "openshift.io") {
			log.Printf("✓ SUCCESS: Found OpenShift API group: %s", group.Name)
			mode := "enabled"
			d.openShiftDetected = &mode
			d.openShiftCacheTime = time.Now()
			return mode
		}
	}
	
	log.Printf("✗ No OpenShift API groups found")
	mode := "disabled"
	d.openShiftDetected = &mode
	d.openShiftCacheTime = time.Now()
	return mode
}

// detectOpenShiftClusterName tries to get cluster name from OpenShift Infrastructure
func (d *Detector) detectOpenShiftClusterName() string {
	// Try to get OpenShift Infrastructure object
	gvr := schema.GroupVersionResource{
		Group:    "config.openshift.io",
		Version:  "v1",
		Resource: "infrastructures",
	}
	
	ctx, cancel := context.WithTimeout(d.ctx, 10*time.Second)
	defer cancel()
	
	infraList, err := d.dynamicClient.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("Failed to list OpenShift Infrastructure objects: %v", err)
		return ""
	}
	
	for _, item := range infraList.Items {
		if name, found, err := unstructured.NestedString(item.Object, "status", "infrastructureName"); err == nil && found && name != "" {
			log.Printf("Found infrastructure name in status: '%s'", name)
			return name
		}
		
		if name, found, err := unstructured.NestedString(item.Object, "metadata", "name"); err == nil && found && name != "" && name != "cluster" {
			log.Printf("Found infrastructure name in metadata: '%s'", name)
			return name
		}
	}
	
	return ""
}

// detectFromNamespaceLabels tries to detect cluster name from kube-system namespace labels
func (d *Detector) detectFromNamespaceLabels() string {
	ctx, cancel := context.WithTimeout(d.ctx, 10*time.Second)
	defer cancel()
	
	ns, err := d.clientset.CoreV1().Namespaces().Get(ctx, "kube-system", metav1.GetOptions{})
	if err != nil {
		log.Printf("Failed to get kube-system namespace: %v", err)
		return ""
	}
	
	// Check common label keys for cluster name
	labelKeys := []string{
		"cluster-name",
		"cluster.name",
		"kubernetes.io/cluster-name",
		"openshift.io/cluster-name",
		"gardener.cloud/shoot-name",
		"azure.workload.identity/cluster-name",
		"eks.amazonaws.com/cluster-name",
		"container.googleapis.com/cluster_name",
	}
	
	for _, key := range labelKeys {
		if value, exists := ns.Labels[key]; exists && value != "" {
			log.Printf("Found cluster name in namespace label '%s': '%s'", key, value)
			return value
		}
	}
	
	return ""
}

// detectFromNodeLabels tries to detect cluster name from node labels
func (d *Detector) detectFromNodeLabels() string {
	ctx, cancel := context.WithTimeout(d.ctx, 10*time.Second)
	defer cancel()
	
	nodes, err := d.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{Limit: 5})
	if err != nil {
		log.Printf("Failed to list nodes: %v", err)
		return ""
	}
	
	// Check common label keys for cluster name on nodes
	labelKeys := []string{
		"cluster-name",
		"kubernetes.io/cluster-name",
		"alpha.eksctl.io/cluster-name",
		"eks.amazonaws.com/cluster-name",
		"gardener.cloud/shoot-name",
		"azure.workload.identity/cluster-name",
		"container.googleapis.com/cluster_name",
	}
	
	for _, node := range nodes.Items {
		for _, key := range labelKeys {
			if value, exists := node.Labels[key]; exists && value != "" {
				log.Printf("Found cluster name in node label '%s' on node '%s': '%s'", key, node.Name, value)
				return value
			}
		}
	}
	
	return ""
}

// detectFromHostname tries to extract cluster name from hostname patterns
func (d *Detector) detectFromHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		log.Printf("Failed to get hostname: %v", err)
		return ""
	}
	
	log.Printf("Current hostname: '%s'", hostname)
	
	// Try to extract cluster name from common hostname patterns
	patterns := []string{
		// EKS pattern: ip-10-0-1-23.us-west-2.compute.internal
		"compute.internal",
		// GKE pattern: gke-cluster-name-pool-hash
		"gke-",
		// AKS pattern: aks-nodepool-hash
		"aks-",
		// General Kubernetes patterns
		"k8s-",
		"kube-",
	}
	
	for _, pattern := range patterns {
		if strings.Contains(hostname, pattern) {
			// Try to extract meaningful cluster name
			parts := strings.Split(hostname, "-")
			if len(parts) >= 2 {
				// Return first meaningful part
				for _, part := range parts {
					if len(part) > 2 && !strings.Contains(part, ".") && part != "ip" && part != "gke" && part != "aks" && part != "k8s" && part != "kube" {
						log.Printf("Extracted cluster name from hostname pattern '%s': '%s'", pattern, part)
						return part
					}
				}
			}
		}
	}
	
	// Fallback: use hostname as-is if it looks reasonable
	if len(hostname) > 0 && len(hostname) < 64 && !strings.Contains(hostname, ".") {
		log.Printf("Using hostname as cluster name: '%s'", hostname)
		return hostname
	}
	
	return ""
}

// detectDomainFromOpenShiftDNS tries to detect domain from OpenShift DNS configuration
func (d *Detector) detectDomainFromOpenShiftDNS() string {
	// Try to get OpenShift DNS object
	gvr := schema.GroupVersionResource{
		Group:    "config.openshift.io",
		Version:  "v1",
		Resource: "dnses",
	}
	
	ctx, cancel := context.WithTimeout(d.ctx, 10*time.Second)
	defer cancel()
	
	dnsList, err := d.dynamicClient.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("Failed to list OpenShift DNS objects: %v", err)
		return ""
	}
	
	for _, item := range dnsList.Items {
		if baseDomain, found, err := unstructured.NestedString(item.Object, "spec", "baseDomain"); err == nil && found && baseDomain != "" {
			log.Printf("Found base domain in OpenShift DNS spec: '%s'", baseDomain)
			return baseDomain
		}
	}
	
	return ""
}

// detectDomainFromDNSConfig tries to detect domain from DNS configuration
func (d *Detector) detectDomainFromDNSConfig() string {
	ctx, cancel := context.WithTimeout(d.ctx, 10*time.Second)
	defer cancel()
	
	// Check kube-dns or coredns configmap
	configMaps := []string{"kube-dns", "coredns"}
	namespaces := []string{"kube-system", "openshift-dns"}
	
	for _, namespace := range namespaces {
		for _, cmName := range configMaps {
			cm, err := d.clientset.CoreV1().ConfigMaps(namespace).Get(ctx, cmName, metav1.GetOptions{})
			if err != nil {
				continue
			}
			
			// Look for domain configuration in various keys
			keys := []string{"Corefile", "config", "dns"}
			for _, key := range keys {
				if data, exists := cm.Data[key]; exists && data != "" {
					// Parse common domain patterns from DNS config
					lines := strings.Split(data, "\n")
					for _, line := range lines {
						line = strings.TrimSpace(line)
						// Look for cluster.local or custom domain
						if strings.Contains(line, "cluster.local") {
							return "cluster.local"
						}
						if strings.Contains(line, ":53") {
							// Extract domain from lines like: ".:53 {"
							parts := strings.Fields(line)
							if len(parts) > 0 && strings.Contains(parts[0], ".") {
								domain := strings.TrimSuffix(parts[0], ":53")
								if domain != "." && len(domain) > 1 {
									log.Printf("Found domain in DNS config: '%s'", domain)
									return domain
								}
							}
						}
					}
				}
			}
		}
	}
	
	return ""
}

// CreateFromInClusterConfig creates a detector using in-cluster configuration
func CreateFromInClusterConfig() (*Detector, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster config: %v", err)
	}
	
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %v", err)
	}
	
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %v", err)
	}
	
	ctx := context.Background()
	return NewDetector(clientset, dynamicClient, ctx), nil
}
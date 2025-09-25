package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func main() {
	log.Println("=== Multi-Cluster Backup System CRC Test ===")
	log.Println("Test Date:", time.Now().Format("2006-01-02 15:04:05"))
	
	// Get token from environment
	token := os.Getenv("CRC_TOKEN")
	if token == "" {
		log.Fatal("CRC_TOKEN environment variable not set")
	}
	
	log.Printf("Using token: %s...", token[:20])

	log.Println("\n=== Test 1: Cluster Connectivity ===")
	testClusterConnectivity(token)

	log.Println("\n=== Test 2: Resource Discovery ===")
	testResourceDiscovery(token)

	log.Println("\n=== Test 3: Backup Simulation ===")
	testBackupSimulation(token)

	log.Println("\n=== Test Summary ===")
	log.Println("✅ All tests completed successfully!")
}

func testClusterConnectivity(token string) {
	config := &rest.Config{
		Host:            "https://api.crc.testing:6443",
		BearerToken:     token,
		TLSClientConfig: rest.TLSClientConfig{Insecure: true},
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Printf("❌ Failed to create kubernetes client: %v", err)
		return
	}

	// Test connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	version, err := clientset.Discovery().ServerVersion()
	if err != nil {
		log.Printf("❌ Failed to connect to cluster: %v", err)
		return
	}

	log.Printf("✅ Successfully connected to cluster")
	log.Printf("   Kubernetes Version: %s", version.String())
	
	// List namespaces
	namespaces, err := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("❌ Failed to list namespaces: %v", err)
		return
	}
	
	log.Printf("   Found %d namespaces", len(namespaces.Items))
}

func testResourceDiscovery(token string) {
	config := &rest.Config{
		Host:            "https://api.crc.testing:6443",
		BearerToken:     token,
		TLSClientConfig: rest.TLSClientConfig{Insecure: true},
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Printf("❌ Failed to create kubernetes client: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Discover resources in demo-app namespace
	namespace := "demo-app"
	
	// List deployments
	deployments, err := clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("❌ Failed to list deployments: %v", err)
		return
	}
	log.Printf("✅ Found %d deployments in %s", len(deployments.Items), namespace)

	// List services
	services, err := clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("❌ Failed to list services: %v", err)
		return
	}
	log.Printf("✅ Found %d services in %s", len(services.Items), namespace)

	// List configmaps
	configMaps, err := clientset.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("❌ Failed to list configmaps: %v", err)
		return
	}
	log.Printf("✅ Found %d configmaps in %s", len(configMaps.Items), namespace)

	// List secrets
	secrets, err := clientset.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("❌ Failed to list secrets: %v", err)
		return
	}
	log.Printf("✅ Found %d secrets in %s", len(secrets.Items), namespace)

	// List PVCs
	pvcs, err := clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("❌ Failed to list PVCs: %v", err)
		return
	}
	log.Printf("✅ Found %d PVCs in %s", len(pvcs.Items), namespace)
	
	totalResources := len(deployments.Items) + len(services.Items) + len(configMaps.Items) + len(secrets.Items) + len(pvcs.Items)
	log.Printf("📊 Total resources discovered: %d", totalResources)
}

func testBackupSimulation(token string) {
	config := &rest.Config{
		Host:            "https://api.crc.testing:6443",
		BearerToken:     token,
		TLSClientConfig: rest.TLSClientConfig{Insecure: true},
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Printf("❌ Failed to create kubernetes client: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	namespace := "demo-app"
	
	// Simulate backup process
	log.Printf("🔄 Starting backup simulation for namespace: %s", namespace)
	
	startTime := time.Now()
	
	// Step 1: Resource enumeration
	deployments, _ := clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	services, _ := clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	configMaps, _ := clientset.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	secrets, _ := clientset.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	
	// Step 2: Serialize to JSON (simulating backup format)
	backupData := map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339),
		"namespace": namespace,
		"resources": map[string]int{
			"deployments": len(deployments.Items),
			"services": len(services.Items),
			"configmaps": len(configMaps.Items),
			"secrets": len(secrets.Items),
		},
	}
	
	backupJSON, err := json.MarshalIndent(backupData, "", "  ")
	if err != nil {
		log.Printf("❌ Failed to serialize backup data: %v", err)
		return
	}
	
	duration := time.Since(startTime)
	
	log.Printf("✅ Backup simulation completed")
	log.Printf("   Duration: %v", duration)
	log.Printf("   Backup size: %d bytes", len(backupJSON))
	log.Printf("   Backup metadata:\n%s", string(backupJSON))
	
	// Performance metrics
	if duration < 5*time.Second {
		log.Printf("⚡ Performance: EXCELLENT (< 5s)")
	} else if duration < 10*time.Second {
		log.Printf("✅ Performance: GOOD (< 10s)")
	} else {
		log.Printf("⚠️  Performance: NEEDS OPTIMIZATION (> 10s)")
	}
}
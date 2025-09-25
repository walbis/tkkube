package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"
)

func main() {
	log.Println("=== Simple Backup Test: CRC Resources to Local Files ===")
	
	token := os.Getenv("CRC_TOKEN")
	if token == "" {
		log.Fatal("CRC_TOKEN environment variable not set")
	}

	// Test 1: Kubernetes connectivity
	log.Println("\n🔗 Test 1: Kubernetes Connectivity")
	kubeClient, err := createKubeClient(token)
	if err != nil {
		log.Fatalf("Failed to create kubernetes client: %v", err)
	}
	log.Println("✅ Kubernetes client created successfully")

	// Test 2: Resource enumeration and local backup
	log.Println("\n📋 Test 2: Resource Discovery and Local Backup")
	err = backupToLocalFiles(kubeClient, "demo-app")
	if err != nil {
		log.Fatalf("Failed to backup resources: %v", err)
	}

	// Test 3: MinIO connectivity test
	log.Println("\n🗄️  Test 3: MinIO Connectivity Test")
	err = testMinIOConnectivity()
	if err != nil {
		log.Printf("⚠️  MinIO connectivity test failed: %v", err)
		log.Println("This is expected if MinIO is not accessible from outside cluster")
	} else {
		log.Println("✅ MinIO connectivity successful")
	}

	log.Println("\n✅ Simple backup test completed!")
}

func createKubeClient(token string) (*kubernetes.Clientset, error) {
	config := &rest.Config{
		Host:            "https://api.crc.testing:6443",
		BearerToken:     token,
		TLSClientConfig: rest.TLSClientConfig{Insecure: true},
	}

	return kubernetes.NewForConfig(config)
}

func backupToLocalFiles(kubeClient *kubernetes.Clientset, namespace string) error {
	ctx := context.Background()

	// Create backup directory
	backupDir := fmt.Sprintf("backup_%s_%s", namespace, time.Now().Format("2006-01-02_15-04-05"))
	err := os.MkdirAll(backupDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	log.Printf("📁 Created backup directory: %s", backupDir)

	// Backup deployments
	deployments, err := kubeClient.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list deployments: %w", err)
	}

	if len(deployments.Items) > 0 {
		yamlData, err := yaml.Marshal(deployments.Items)
		if err != nil {
			return fmt.Errorf("failed to marshal deployments: %w", err)
		}

		filename := fmt.Sprintf("%s/deployments.yaml", backupDir)
		err = os.WriteFile(filename, yamlData, 0644)
		if err != nil {
			return fmt.Errorf("failed to write deployments file: %w", err)
		}
		log.Printf("📤 Backed up %d deployments to %s", len(deployments.Items), filename)
	}

	// Backup services
	services, err := kubeClient.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list services: %w", err)
	}

	if len(services.Items) > 0 {
		yamlData, err := yaml.Marshal(services.Items)
		if err != nil {
			return fmt.Errorf("failed to marshal services: %w", err)
		}

		filename := fmt.Sprintf("%s/services.yaml", backupDir)
		err = os.WriteFile(filename, yamlData, 0644)
		if err != nil {
			return fmt.Errorf("failed to write services file: %w", err)
		}
		log.Printf("📤 Backed up %d services to %s", len(services.Items), filename)
	}

	// Backup configmaps
	configMaps, err := kubeClient.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list configmaps: %w", err)
	}

	if len(configMaps.Items) > 0 {
		yamlData, err := yaml.Marshal(configMaps.Items)
		if err != nil {
			return fmt.Errorf("failed to marshal configmaps: %w", err)
		}

		filename := fmt.Sprintf("%s/configmaps.yaml", backupDir)
		err = os.WriteFile(filename, yamlData, 0644)
		if err != nil {
			return fmt.Errorf("failed to write configmaps file: %w", err)
		}
		log.Printf("📤 Backed up %d configmaps to %s", len(configMaps.Items), filename)
	}

	// Create backup summary
	summary := map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339),
		"cluster":   "crc-cluster",
		"namespace": namespace,
		"resources": map[string]int{
			"deployments": len(deployments.Items),
			"services":    len(services.Items),
			"configmaps":  len(configMaps.Items),
		},
		"backup_directory": backupDir,
	}

	summaryYAML, err := yaml.Marshal(summary)
	if err != nil {
		return fmt.Errorf("failed to marshal summary: %w", err)
	}

	summaryFile := fmt.Sprintf("%s/backup-summary.yaml", backupDir)
	err = os.WriteFile(summaryFile, summaryYAML, 0644)
	if err != nil {
		return fmt.Errorf("failed to write summary file: %w", err)
	}

	log.Printf("📋 Created backup summary: %s", summaryFile)
	return nil
}

func testMinIOConnectivity() error {
	// Try multiple MinIO endpoints
	endpoints := []string{
		"localhost:9001",
		"localhost:9000",
		"minio.backup-storage.svc.cluster.local:9000",
	}

	for _, endpoint := range endpoints {
		log.Printf("🔍 Testing MinIO endpoint: %s", endpoint)
		
		minioClient, err := minio.New(endpoint, &minio.Options{
			Creds:  credentials.NewStaticV4("backup", "backup123", ""),
			Secure: false,
		})
		if err != nil {
			log.Printf("❌ Failed to create client for %s: %v", endpoint, err)
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Try to list buckets
		_, err = minioClient.ListBuckets(ctx)
		if err != nil {
			log.Printf("❌ Failed to connect to %s: %v", endpoint, err)
			continue
		}

		log.Printf("✅ Successfully connected to MinIO at %s", endpoint)
		return nil
	}

	return fmt.Errorf("failed to connect to any MinIO endpoint")
}
package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"
)

type BackupExecutor struct {
	kubeClient  *kubernetes.Clientset
	minioClient *minio.Client
	bucketName  string
	backupPath  string
}

type BackupManifest struct {
	Timestamp    string                 `yaml:"timestamp"`
	Cluster      string                 `yaml:"cluster"`
	Namespace    string                 `yaml:"namespace"`
	Resources    map[string]int         `yaml:"resources"`
	Files        []string               `yaml:"files"`
	TotalSize    int64                  `yaml:"totalSize"`
	BackupPath   string                 `yaml:"backupPath"`
	Metadata     map[string]interface{} `yaml:"metadata"`
}

func main() {
	log.Println("=== Real Backup Execution: CRC to MinIO (In-Cluster) ===")
	log.Println("Starting actual backup operation...")

	// Get authentication token
	token := os.Getenv("CRC_TOKEN")
	if token == "" {
		log.Fatal("CRC_TOKEN environment variable not set")
	}

	// Initialize backup executor
	executor, err := NewBackupExecutor(token)
	if err != nil {
		log.Fatalf("Failed to initialize backup executor: %v", err)
	}

	// Execute real backup
	manifest, err := executor.ExecuteBackup("demo-app")
	if err != nil {
		log.Fatalf("Backup execution failed: %v", err)
	}

	// Display results
	executor.DisplayResults(manifest)
	
	log.Println("✅ Real backup execution completed successfully!")
}

func NewBackupExecutor(token string) (*BackupExecutor, error) {
	// Initialize Kubernetes client (in-cluster config)
	config := &rest.Config{
		Host:            "https://api.crc.testing:6443",
		BearerToken:     token,
		TLSClientConfig: rest.TLSClientConfig{Insecure: true},
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Initialize MinIO client using internal cluster DNS
	minioClient, err := minio.New("minio.backup-storage.svc.cluster.local:9000", &minio.Options{
		Creds:  credentials.NewStaticV4("backup", "backup123", ""),
		Secure: false,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	return &BackupExecutor{
		kubeClient:  kubeClient,
		minioClient: minioClient,
		bucketName:  "cluster-backups",
		backupPath:  fmt.Sprintf("crc-cluster/%s", time.Now().Format("2006-01-02_15-04-05")),
	}, nil
}

func (be *BackupExecutor) ExecuteBackup(namespace string) (*BackupManifest, error) {
	ctx := context.Background()
	
	log.Printf("🔄 Starting backup for namespace: %s", namespace)
	
	// Ensure bucket exists
	exists, err := be.minioClient.BucketExists(ctx, be.bucketName)
	if err != nil {
		return nil, fmt.Errorf("error checking bucket: %w", err)
	}
	
	if !exists {
		log.Printf("📦 Creating bucket: %s", be.bucketName)
		err = be.minioClient.MakeBucket(ctx, be.bucketName, minio.MakeBucketOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to create bucket: %w", err)
		}
	}

	manifest := &BackupManifest{
		Timestamp:  time.Now().Format(time.RFC3339),
		Cluster:    "crc-cluster",
		Namespace:  namespace,
		Resources:  make(map[string]int),
		Files:      []string{},
		BackupPath: be.backupPath,
		Metadata: map[string]interface{}{
			"method":  "cluster-backup-executor",
			"version": "1.0.0",
		},
	}

	var totalSize int64

	// Backup Deployments
	deployments, err := be.backupDeployments(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to backup deployments: %w", err)
	}
	if len(deployments) > 0 {
		size, err := be.uploadResourceYAML(ctx, "deployments.yaml", deployments, namespace)
		if err != nil {
			return nil, err
		}
		manifest.Resources["deployments"] = len(deployments)
		manifest.Files = append(manifest.Files, "deployments.yaml")
		totalSize += size
	}

	// Backup Services
	services, err := be.backupServices(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to backup services: %w", err)
	}
	if len(services) > 0 {
		size, err := be.uploadResourceYAML(ctx, "services.yaml", services, namespace)
		if err != nil {
			return nil, err
		}
		manifest.Resources["services"] = len(services)
		manifest.Files = append(manifest.Files, "services.yaml")
		totalSize += size
	}

	// Backup ConfigMaps
	configMaps, err := be.backupConfigMaps(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to backup configmaps: %w", err)
	}
	if len(configMaps) > 0 {
		size, err := be.uploadResourceYAML(ctx, "configmaps.yaml", configMaps, namespace)
		if err != nil {
			return nil, err
		}
		manifest.Resources["configmaps"] = len(configMaps)
		manifest.Files = append(manifest.Files, "configmaps.yaml")
		totalSize += size
	}

	// Backup Secrets
	secrets, err := be.backupSecrets(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to backup secrets: %w", err)
	}
	if len(secrets) > 0 {
		size, err := be.uploadResourceYAML(ctx, "secrets.yaml", secrets, namespace)
		if err != nil {
			return nil, err
		}
		manifest.Resources["secrets"] = len(secrets)
		manifest.Files = append(manifest.Files, "secrets.yaml")
		totalSize += size
	}

	// Backup PVCs
	pvcs, err := be.backupPVCs(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to backup PVCs: %w", err)
	}
	if len(pvcs) > 0 {
		size, err := be.uploadResourceYAML(ctx, "pvcs.yaml", pvcs, namespace)
		if err != nil {
			return nil, err
		}
		manifest.Resources["pvcs"] = len(pvcs)
		manifest.Files = append(manifest.Files, "pvcs.yaml")
		totalSize += size
	}

	manifest.TotalSize = totalSize

	// Upload backup manifest
	manifestSize, err := be.uploadManifest(ctx, manifest)
	if err != nil {
		return nil, fmt.Errorf("failed to upload manifest: %w", err)
	}
	manifest.TotalSize += manifestSize

	log.Printf("✅ Backup completed for namespace: %s", namespace)
	return manifest, nil
}

func (be *BackupExecutor) backupDeployments(ctx context.Context, namespace string) ([]appsv1.Deployment, error) {
	deploymentsList, err := be.kubeClient.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	
	log.Printf("📋 Found %d deployments in namespace %s", len(deploymentsList.Items), namespace)
	return deploymentsList.Items, nil
}

func (be *BackupExecutor) backupServices(ctx context.Context, namespace string) ([]corev1.Service, error) {
	servicesList, err := be.kubeClient.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	
	log.Printf("🔗 Found %d services in namespace %s", len(servicesList.Items), namespace)
	return servicesList.Items, nil
}

func (be *BackupExecutor) backupConfigMaps(ctx context.Context, namespace string) ([]corev1.ConfigMap, error) {
	configMapsList, err := be.kubeClient.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	
	log.Printf("⚙️  Found %d configmaps in namespace %s", len(configMapsList.Items), namespace)
	return configMapsList.Items, nil
}

func (be *BackupExecutor) backupSecrets(ctx context.Context, namespace string) ([]corev1.Secret, error) {
	secretsList, err := be.kubeClient.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	
	// Filter out service account tokens and other system secrets
	var filteredSecrets []corev1.Secret
	for _, secret := range secretsList.Items {
		if secret.Type != corev1.SecretTypeServiceAccountToken &&
		   !strings.HasPrefix(secret.Name, "builder-token-") &&
		   !strings.HasPrefix(secret.Name, "default-token-") &&
		   !strings.HasPrefix(secret.Name, "deployer-token-") {
			filteredSecrets = append(filteredSecrets, secret)
		}
	}
	
	log.Printf("🔐 Found %d user secrets in namespace %s (filtered from %d total)", 
		len(filteredSecrets), namespace, len(secretsList.Items))
	return filteredSecrets, nil
}

func (be *BackupExecutor) backupPVCs(ctx context.Context, namespace string) ([]corev1.PersistentVolumeClaim, error) {
	pvcsList, err := be.kubeClient.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	
	log.Printf("💾 Found %d PVCs in namespace %s", len(pvcsList.Items), namespace)
	return pvcsList.Items, nil
}

func (be *BackupExecutor) uploadResourceYAML(ctx context.Context, filename string, resources interface{}, namespace string) (int64, error) {
	// Clean up resources by removing system-managed fields
	cleanedResources := be.cleanResources(resources)
	
	// Serialize to YAML
	yamlData, err := yaml.Marshal(cleanedResources)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal resources to YAML: %w", err)
	}

	// Create object path
	objectPath := filepath.Join(be.backupPath, namespace, filename)
	
	// Upload to MinIO
	reader := bytes.NewReader(yamlData)
	info, err := be.minioClient.PutObject(ctx, be.bucketName, objectPath, reader, int64(len(yamlData)), minio.PutObjectOptions{
		ContentType: "application/yaml",
	})
	if err != nil {
		return 0, fmt.Errorf("failed to upload %s to MinIO: %w", filename, err)
	}

	log.Printf("📤 Uploaded %s (%d bytes) to MinIO: %s", filename, info.Size, objectPath)
	return info.Size, nil
}

func (be *BackupExecutor) uploadManifest(ctx context.Context, manifest *BackupManifest) (int64, error) {
	// Serialize manifest to YAML
	manifestYAML, err := yaml.Marshal(manifest)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal manifest: %w", err)
	}

	// Upload manifest
	objectPath := filepath.Join(be.backupPath, "backup-manifest.yaml")
	reader := bytes.NewReader(manifestYAML)
	info, err := be.minioClient.PutObject(ctx, be.bucketName, objectPath, reader, int64(len(manifestYAML)), minio.PutObjectOptions{
		ContentType: "application/yaml",
	})
	if err != nil {
		return 0, fmt.Errorf("failed to upload manifest: %w", err)
	}

	log.Printf("📋 Uploaded backup manifest (%d bytes) to MinIO: %s", info.Size, objectPath)
	return info.Size, nil
}

func (be *BackupExecutor) cleanResources(resources interface{}) interface{} {
	// This would clean up system-managed fields from Kubernetes resources
	// For now, return as-is - in production, we'd remove fields like:
	// - metadata.resourceVersion
	// - metadata.uid  
	// - metadata.generation
	// - status fields
	// - system annotations and labels
	
	return resources
}

func (be *BackupExecutor) DisplayResults(manifest *BackupManifest) {
	log.Println("\n=== BACKUP EXECUTION RESULTS ===")
	log.Printf("📅 Timestamp: %s", manifest.Timestamp)
	log.Printf("🎯 Cluster: %s", manifest.Cluster)
	log.Printf("📂 Namespace: %s", manifest.Namespace)
	log.Printf("📁 Backup Path: %s", manifest.BackupPath)
	log.Printf("📊 Total Size: %d bytes", manifest.TotalSize)
	
	log.Println("\n📋 Resources Backed Up:")
	for resourceType, count := range manifest.Resources {
		log.Printf("  %s: %d", resourceType, count)
	}
	
	log.Println("\n📄 Files Created:")
	for _, file := range manifest.Files {
		log.Printf("  %s", file)
	}
	
	log.Printf("\n🎯 MinIO Location: s3://cluster-backups/%s/", manifest.BackupPath)
}
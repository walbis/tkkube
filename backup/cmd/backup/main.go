package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"cluster-backup/internal/backup"
	"cluster-backup/internal/config"
	"cluster-backup/internal/logging"
	"cluster-backup/internal/metrics"
)

var (
	version = "dev" // Set by build process
)

func main() {
	var (
		showVersion  = flag.Bool("version", false, "Show version and exit")
		healthCheck  = flag.Bool("health-check", false, "Run health check and exit")
		dryRun       = flag.Bool("dry-run", false, "Perform a dry run without making changes")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("backup version %s\n", version)
		os.Exit(0)
	}

	if *healthCheck {
		if err := performHealthCheck(); err != nil {
			fmt.Printf("Health check failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Health check passed")
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	backupCfg, err := config.LoadBackupConfig()
	if err != nil {
		log.Fatalf("Failed to load backup configuration: %v", err)
	}

	// Initialize logger
	logger := logging.NewStructuredLogger("backup", cfg.ClusterName)
	
	if *dryRun {
		logger.Info("startup", "Starting backup in dry-run mode", map[string]interface{}{
			"version": version,
			"cluster": cfg.ClusterName,
		})
	} else {
		logger.Info("startup", "Starting cluster backup service", map[string]interface{}{
			"version": version,
			"cluster": cfg.ClusterName,
			"bucket":  cfg.MinIOBucket,
		})
	}

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		logger.Info("shutdown", "Received signal, initiating graceful shutdown", map[string]interface{}{
			"signal": sig.String(),
		})
		cancel()
	}()

	// Initialize Kubernetes clients
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		logger.Error("kubernetes_config_failed", "Failed to create Kubernetes config", map[string]interface{}{
			"error": err.Error(),
		})
		os.Exit(1)
	}

	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		logger.Error("kubernetes_client_failed", "Failed to create Kubernetes client", map[string]interface{}{
			"error": err.Error(),
		})
		os.Exit(1)
	}

	dynamicClient, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		logger.Error("dynamic_client_failed", "Failed to create dynamic client", map[string]interface{}{
			"error": err.Error(),
		})
		os.Exit(1)
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(kubeConfig)
	if err != nil {
		logger.Error("discovery_client_failed", "Failed to create discovery client", map[string]interface{}{
			"error": err.Error(),
		})
		os.Exit(1)
	}

	// Initialize MinIO client
	minioClient, err := minio.New(cfg.MinIOEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.MinIOAccessKey, cfg.MinIOSecretKey, ""),
		Secure: cfg.MinIOUseSSL,
	})
	if err != nil {
		logger.Error("minio_client_failed", "Failed to create MinIO client", map[string]interface{}{
			"error":    err.Error(),
			"endpoint": cfg.MinIOEndpoint,
		})
		os.Exit(1)
	}

	// Initialize metrics
	backupMetrics := metrics.NewBackupMetrics()

	// Create backup instance
	clusterBackup := backup.NewClusterBackup(
		cfg,
		backupCfg,
		kubeClient,
		dynamicClient,
		discoveryClient,
		minioClient,
		logger,
		backupMetrics,
		ctx,
	)

	if *dryRun {
		logger.Info("dry_run_complete", "Dry run completed successfully", nil)
		os.Exit(0)
	}

	// Execute backup
	result, err := clusterBackup.ExecuteBackup()
	if err != nil {
		logger.Error("backup_failed", "Backup operation failed", map[string]interface{}{
			"error": err.Error(),
		})
		os.Exit(1)
	}

	logger.Info("backup_success", "Backup operation completed successfully", map[string]interface{}{
		"namespaces_backed_up": result.NamespacesBackedUp,
		"resources_backed_up":  result.ResourcesBackedUp,
		"duration_seconds":     result.Duration.Seconds(),
		"error_count":          len(result.Errors),
	})

	if len(result.Errors) > 0 {
		logger.Warning("backup_errors", "Backup completed with errors", map[string]interface{}{
			"error_count": len(result.Errors),
			"errors":      result.Errors,
		})
		os.Exit(1)
	}
}

// performHealthCheck performs a basic health check
func performHealthCheck() error {
	// Load configuration to verify it's valid
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("configuration validation failed: %v", err)
	}

	// Test MinIO connectivity
	minioClient, err := minio.New(cfg.MinIOEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.MinIOAccessKey, cfg.MinIOSecretKey, ""),
		Secure: cfg.MinIOUseSSL,
	})
	if err != nil {
		return fmt.Errorf("MinIO client creation failed: %v", err)
	}

	ctx := context.Background()
	_, err = minioClient.BucketExists(ctx, cfg.MinIOBucket)
	if err != nil {
		return fmt.Errorf("MinIO connectivity test failed: %v", err)
	}

	return nil
}
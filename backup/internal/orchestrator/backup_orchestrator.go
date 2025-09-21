package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"cluster-backup/internal/backup"
	"cluster-backup/internal/cleanup"
	"cluster-backup/internal/cluster"
	"cluster-backup/internal/config"
	"cluster-backup/internal/logging"
	"cluster-backup/internal/metrics"
	"cluster-backup/internal/priority"
	"cluster-backup/internal/resilience"
	"cluster-backup/internal/server"
)

// BackupOrchestrator coordinates all backup-related operations
type BackupOrchestrator struct {
	// Core components
	config          *config.Config
	backupConfig    *config.BackupConfig
	logger          *logging.StructuredLogger
	ctx             context.Context
	
	// Kubernetes clients
	kubeClient      kubernetes.Interface
	dynamicClient   dynamic.Interface
	discoveryClient discovery.DiscoveryInterface
	
	// MinIO client
	minioClient     *minio.Client
	
	// Specialized managers
	clusterDetector *cluster.Detector
	priorityManager *priority.Manager
	backupManager   *backup.ClusterBackup
	cleanupManager  *cleanup.Manager
	metricsManager  *metrics.BackupMetrics
	metricsServer   *server.MetricsServer
	
	// Resilience components
	minioCircuitBreaker *resilience.CircuitBreaker
	apiCircuitBreaker   *resilience.CircuitBreaker
	retryExecutor       *resilience.RetryExecutor
}

// OrchestratorConfig holds configuration for the orchestrator
type OrchestratorConfig struct {
	MetricsPort        int
	ContextTimeout     time.Duration
	EnableMetricsServer bool
}

// DefaultOrchestratorConfig returns sensible defaults
func DefaultOrchestratorConfig() *OrchestratorConfig {
	return &OrchestratorConfig{
		MetricsPort:         8080,
		ContextTimeout:      30 * time.Minute,
		EnableMetricsServer: true,
	}
}

// NewBackupOrchestrator creates a new backup orchestrator with all components initialized
func NewBackupOrchestrator(orchestratorConfig *OrchestratorConfig) (*BackupOrchestrator, error) {
	if orchestratorConfig == nil {
		orchestratorConfig = DefaultOrchestratorConfig()
	}
	
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %v", err)
	}
	
	backupCfg, err := config.LoadBackupConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load backup config: %v", err)
	}
	
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), orchestratorConfig.ContextTimeout)
	_ = cancel // Keep the cancel function available if needed later
	
	// Initialize logger
	logger := logging.NewStructuredLogger("backup-orchestrator", cfg.ClusterName)
	
	// Create Kubernetes clients
	kubeClient, dynamicClient, discoveryClient, err := createKubernetesClients()
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes clients: %v", err)
	}
	
	// Create MinIO client
	minioClient, err := createMinIOClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %v", err)
	}
	
	// Create cluster detector and update configuration with detected values
	clusterDetector := cluster.NewDetector(kubeClient, dynamicClient, ctx)
	updateConfigWithDetectedValues(cfg, clusterDetector)
	
	// Create specialized managers
	priorityManager := priority.NewManager(kubeClient, "backup-priority-config", "default")
	metricsManager := metrics.NewBackupMetrics()
	
	backupManager := backup.NewClusterBackup(
		cfg,
		backupCfg,
		kubeClient,
		dynamicClient,
		discoveryClient,
		minioClient,
		logger,
		metricsManager,
		ctx,
	)
	
	cleanupManager := cleanup.NewManager(cfg, minioClient, logger, metricsManager, ctx)
	
	// Create resilience components
	minioCircuitBreaker := resilience.NewCircuitBreaker(5, 1*time.Minute)
	apiCircuitBreaker := resilience.NewCircuitBreaker(3, 30*time.Second)
	retryExecutor := resilience.NewRetryExecutor(resilience.DefaultRetryConfig())
	
	// Create metrics server if enabled
	var metricsServer *server.MetricsServer
	if orchestratorConfig.EnableMetricsServer {
		metricsServer = server.NewMetricsServer(orchestratorConfig.MetricsPort, logger)
	}
	
	orchestrator := &BackupOrchestrator{
		config:              cfg,
		backupConfig:        backupCfg,
		logger:              logger,
		ctx:                 ctx,
		kubeClient:          kubeClient,
		dynamicClient:       dynamicClient,
		discoveryClient:     discoveryClient,
		minioClient:         minioClient,
		clusterDetector:     clusterDetector,
		priorityManager:     priorityManager,
		backupManager:       backupManager,
		cleanupManager:      cleanupManager,
		metricsManager:      metricsManager,
		metricsServer:       metricsServer,
		minioCircuitBreaker: minioCircuitBreaker,
		apiCircuitBreaker:   apiCircuitBreaker,
		retryExecutor:       retryExecutor,
	}
	
	// Load priority configuration
	if err := priorityManager.LoadConfig(); err != nil {
		logger.Warning("priority_config_load_failed", "Failed to load priority configuration, using defaults", map[string]interface{}{
			"error": err.Error(),
		})
	}
	
	return orchestrator, nil
}

// Run executes the complete backup workflow
func (bo *BackupOrchestrator) Run() error {
	bo.logger.Info("orchestrator_start", "Starting backup orchestration", map[string]interface{}{
		"cluster":   bo.config.ClusterName,
		"bucket":    bo.config.MinIOBucket,
		"retention": bo.config.RetentionDays,
	})
	
	// Start metrics server if configured
	if bo.metricsServer != nil {
		errChan := bo.metricsServer.StartAsync()
		
		// Check for startup errors (non-blocking)
		select {
		case err := <-errChan:
			bo.logger.Error("metrics_server_startup_failed", "Metrics server failed to start", map[string]interface{}{
				"error": err.Error(),
			})
			// Continue with backup even if metrics server fails
		case <-time.After(2 * time.Second):
			bo.logger.Info("metrics_server_started", "Metrics server started successfully", map[string]interface{}{
				"port": bo.metricsServer.GetPort(),
			})
		}
	}
	
	// Perform startup cleanup if configured
	if bo.cleanupManager.ShouldCleanupOnStartup() {
		bo.logger.Info("cleanup_startup", "Performing cleanup on startup", nil)
		if err := bo.performCleanupWithResilience(); err != nil {
			bo.logger.Error("cleanup_startup_failed", "Startup cleanup failed", map[string]interface{}{
				"error": err.Error(),
			})
			// Don't fail the backup if cleanup fails
		}
	}
	
	// Execute backup with resilience
	backupResult, err := bo.executeBackupWithResilience()
	if err != nil {
		return fmt.Errorf("backup execution failed: %v", err)
	}
	
	bo.logger.Info("backup_result", "Backup completed", map[string]interface{}{
		"namespaces_backed_up": backupResult.NamespacesBackedUp,
		"resources_backed_up":  backupResult.ResourcesBackedUp,
		"duration_seconds":     backupResult.Duration.Seconds(),
		"error_count":          len(backupResult.Errors),
	})
	
	// Perform post-backup cleanup if configured
	if bo.cleanupManager.ShouldCleanupAfterBackup() {
		bo.logger.Info("cleanup_post_backup", "Performing cleanup after backup", nil)
		if err := bo.performCleanupWithResilience(); err != nil {
			bo.logger.Error("cleanup_post_backup_failed", "Post-backup cleanup failed", map[string]interface{}{
				"error": err.Error(),
			})
			// Don't fail the overall operation if cleanup fails
		}
	}
	
	bo.logger.Info("orchestrator_complete", "Backup orchestration completed successfully", nil)
	return nil
}

// executeBackupWithResilience executes the backup with circuit breaker and retry protection
func (bo *BackupOrchestrator) executeBackupWithResilience() (*backup.BackupResult, error) {
	var result *backup.BackupResult
	var err error
	
	// Execute backup with retry logic
	retryErr := bo.retryExecutor.ExecuteWithContext(bo.ctx, func() error {
		// Execute backup with MinIO circuit breaker protection
		execErr := bo.minioCircuitBreaker.Execute(func() error {
			result, err = bo.backupManager.ExecuteBackup()
			return err
		})
		
		if resilience.IsCircuitBreakerError(execErr) {
			bo.logger.Error("backup_circuit_breaker_open", "MinIO circuit breaker is open", map[string]interface{}{
				"circuit_state": "open",
			})
			return execErr
		}
		
		return execErr
	})
	
	if retryErr != nil {
		if resilience.IsRetryExhaustedError(retryErr) {
			bo.logger.Error("backup_retry_exhausted", "Backup retry attempts exhausted", map[string]interface{}{
				"error": retryErr.Error(),
			})
		}
		return nil, retryErr
	}
	
	return result, nil
}

// performCleanupWithResilience executes cleanup with circuit breaker protection
func (bo *BackupOrchestrator) performCleanupWithResilience() error {
	return bo.minioCircuitBreaker.Execute(func() error {
		_, err := bo.cleanupManager.PerformCleanup()
		return err
	})
}

// GetClusterInfo returns detected cluster information
func (bo *BackupOrchestrator) GetClusterInfo() *cluster.DetectionResult {
	return bo.clusterDetector.DetectClusterInfo()
}

// GetRetentionInfo returns information about the retention policy
func (bo *BackupOrchestrator) GetRetentionInfo() map[string]interface{} {
	return bo.cleanupManager.GetRetentionInfo()
}

// EstimateCleanupImpact estimates the impact of running cleanup
func (bo *BackupOrchestrator) EstimateCleanupImpact() (*cleanup.CleanupEstimate, error) {
	return bo.cleanupManager.EstimateCleanupImpact()
}

// GetCircuitBreakerStats returns statistics about circuit breakers
func (bo *BackupOrchestrator) GetCircuitBreakerStats() map[string]resilience.CircuitBreakerStats {
	return map[string]resilience.CircuitBreakerStats{
		"minio": bo.minioCircuitBreaker.GetStats(),
		"api":   bo.apiCircuitBreaker.GetStats(),
	}
}

// Shutdown gracefully shuts down the orchestrator
func (bo *BackupOrchestrator) Shutdown(ctx context.Context) error {
	bo.logger.Info("orchestrator_shutdown", "Shutting down backup orchestrator", nil)
	
	if bo.metricsServer != nil {
		if err := bo.metricsServer.Stop(ctx); err != nil {
			bo.logger.Error("metrics_server_shutdown_failed", "Failed to shutdown metrics server", map[string]interface{}{
				"error": err.Error(),
			})
		}
	}
	
	bo.logger.Info("orchestrator_shutdown_complete", "Backup orchestrator shutdown complete", nil)
	return nil
}

// createKubernetesClients creates and returns Kubernetes clients
func createKubernetesClients() (kubernetes.Interface, dynamic.Interface, discovery.DiscoveryInterface, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get in-cluster config: %v", err)
	}
	
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create Kubernetes client: %v", err)
	}
	
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create dynamic client: %v", err)
	}
	
	discoveryClient := kubeClient.Discovery()
	
	return kubeClient, dynamicClient, discoveryClient, nil
}

// createMinIOClient creates and returns a MinIO client
func createMinIOClient(cfg *config.Config) (*minio.Client, error) {
	minioClient, err := minio.New(cfg.MinIOEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.MinIOAccessKey, cfg.MinIOSecretKey, ""),
		Secure: cfg.MinIOUseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %v", err)
	}
	
	return minioClient, nil
}

// updateConfigWithDetectedValues updates configuration with cluster detection results
func updateConfigWithDetectedValues(cfg *config.Config, detector *cluster.Detector) {
	if cfg.ClusterName == "" {
		cfg.ClusterName = detector.DetectClusterName()
	}
	
	if cfg.ClusterDomain == "" {
		cfg.ClusterDomain = detector.DetectClusterDomain()
	}
}
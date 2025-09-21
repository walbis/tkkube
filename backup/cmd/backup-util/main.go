package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"cluster-backup/internal/cluster"
	"cluster-backup/internal/config"
	"cluster-backup/internal/orchestrator"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	
	switch command {
	case "cluster-info":
		showClusterInfo()
	case "config-validate":
		validateConfiguration()
	case "estimate-cleanup":
		estimateCleanup()
	case "circuit-breaker-status":
		showCircuitBreakerStatus()
	case "health-check":
		fmt.Println("OK")
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Backup Utility Commands:")
	fmt.Println("  cluster-info          - Show detected cluster information")
	fmt.Println("  config-validate       - Validate configuration")
	fmt.Println("  estimate-cleanup      - Estimate cleanup impact without performing cleanup")
	fmt.Println("  circuit-breaker-status - Show circuit breaker status")
	fmt.Println("  health-check          - Simple health check")
}

func showClusterInfo() {
	detector, err := cluster.CreateFromInClusterConfig()
	if err != nil {
		log.Fatalf("Failed to create cluster detector: %v", err)
	}

	info := detector.DetectClusterInfo()
	
	fmt.Println("=== Cluster Information ===")
	fmt.Printf("Cluster Name:   %s\n", info.ClusterName)
	fmt.Printf("Cluster Domain: %s\n", info.ClusterDomain)
	fmt.Printf("OpenShift:      %v\n", info.IsOpenShift)
	fmt.Printf("OpenShift Mode: %s\n", info.OpenShiftMode)
}

func validateConfiguration() {
	fmt.Println("=== Configuration Validation ===")
	
	// Load and validate main config
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Printf("❌ Main configuration invalid: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ Main configuration valid")
	
	// Load and validate backup config
	backupCfg, err := config.LoadBackupConfig()
	if err != nil {
		fmt.Printf("❌ Backup configuration invalid: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ Backup configuration valid")
	
	// Show key configuration values
	fmt.Printf("Cluster Name:     %s\n", cfg.ClusterName)
	fmt.Printf("MinIO Endpoint:   %s\n", cfg.MinIOEndpoint)
	fmt.Printf("MinIO Bucket:     %s\n", cfg.MinIOBucket)
	fmt.Printf("Retention Days:   %d\n", cfg.RetentionDays)
	fmt.Printf("Batch Size:       %d\n", cfg.BatchSize)
	fmt.Printf("OpenShift Mode:   %s\n", backupCfg.OpenShiftMode)
	fmt.Printf("Cleanup Enabled:  %v\n", cfg.EnableCleanup)
}

func estimateCleanup() {
	fmt.Println("=== Cleanup Impact Estimation ===")
	
	config := orchestrator.DefaultOrchestratorConfig()
	config.EnableMetricsServer = false // Don't start metrics server for utility
	
	backupOrchestrator, err := orchestrator.NewBackupOrchestrator(config)
	if err != nil {
		log.Fatalf("Failed to create backup orchestrator: %v", err)
	}
	
	estimate, err := backupOrchestrator.EstimateCleanupImpact()
	if err != nil {
		log.Fatalf("Failed to estimate cleanup impact: %v", err)
	}
	
	summary := estimate.GetSummary()
	
	fmt.Printf("Total Files:          %v\n", summary["total_files"])
	fmt.Printf("Files to Delete:      %v\n", summary["files_to_delete"])
	fmt.Printf("Files to Keep:        %v\n", summary["files_to_keep"])
	fmt.Printf("Total Size (MB):      %v\n", summary["total_size_mb"])
	fmt.Printf("Space to Free (MB):   %v\n", summary["space_to_free_mb"])
	fmt.Printf("Retention Days:       %v\n", summary["retention_days"])
	fmt.Printf("Cutoff Time:          %v\n", summary["cutoff_time"])
	
	if oldestAge, ok := summary["oldest_file_age_days"]; ok {
		fmt.Printf("Oldest File Age:      %v days\n", oldestAge)
	}
}

func showCircuitBreakerStatus() {
	fmt.Println("=== Circuit Breaker Status ===")
	
	config := orchestrator.DefaultOrchestratorConfig()
	config.EnableMetricsServer = false // Don't start metrics server for utility
	
	backupOrchestrator, err := orchestrator.NewBackupOrchestrator(config)
	if err != nil {
		log.Fatalf("Failed to create backup orchestrator: %v", err)
	}
	
	stats := backupOrchestrator.GetCircuitBreakerStats()
	
	for name, stat := range stats {
		fmt.Printf("%s Circuit Breaker:\n", name)
		fmt.Printf("  State:         %s\n", stat.State.String())
		fmt.Printf("  Failures:      %d/%d\n", stat.Failures, stat.MaxFailures)
		fmt.Printf("  Success Count: %d\n", stat.SuccessCount)
		fmt.Printf("  Reset Timeout: %v\n", stat.ResetTimeout)
		if !stat.LastFailTime.IsZero() {
			fmt.Printf("  Last Fail:     %v (%v ago)\n", 
				stat.LastFailTime.Format(time.RFC3339),
				time.Since(stat.LastFailTime).Round(time.Second))
		}
		fmt.Println()
	}
}
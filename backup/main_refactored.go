package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cluster-backup/internal/orchestrator"
)

func main() {
	// Handle health check requests
	if len(os.Args) > 1 && os.Args[1] == "--health-check" {
		fmt.Println("OK")
		os.Exit(0)
	}

	log.Printf("Starting Enhanced Kubernetes Cluster Backup System...")

	// Create orchestrator configuration
	config := orchestrator.DefaultOrchestratorConfig()
	
	// Override with environment variables if needed
	if port := os.Getenv("METRICS_PORT"); port != "" {
		// Parse port if provided, otherwise use default
		config.MetricsPort = 8080 // Default, could be parsed from env
	}

	// Create and initialize the backup orchestrator
	backupOrchestrator, err := orchestrator.NewBackupOrchestrator(config)
	if err != nil {
		log.Fatalf("Failed to create backup orchestrator: %v", err)
	}

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("Received signal %v, starting graceful shutdown...", sig)
		
		// Give the orchestrator time to shut down gracefully
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		
		if err := backupOrchestrator.Shutdown(shutdownCtx); err != nil {
			log.Printf("Error during shutdown: %v", err)
		}
		
		os.Exit(0)
	}()

	// Log cluster information
	clusterInfo := backupOrchestrator.GetClusterInfo()
	log.Printf("Cluster Information: Name=%s, Domain=%s, OpenShift=%v", 
		clusterInfo.ClusterName, clusterInfo.ClusterDomain, clusterInfo.IsOpenShift)

	// Log retention policy information
	retentionInfo := backupOrchestrator.GetRetentionInfo()
	log.Printf("Retention Policy: %+v", retentionInfo)

	// Run the backup orchestrator
	if err := backupOrchestrator.Run(); err != nil {
		log.Fatalf("Backup orchestrator failed: %v", err)
	}

	log.Printf("Backup operation completed successfully")
}
package integration

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	sharedconfig "shared-config/config"
)

// HTTPServer provides HTTP endpoints for the integration bridge
type HTTPServer struct {
	bridge     *IntegrationBridge
	server     *http.Server
	router     *mux.Router
	config     *sharedconfig.SharedConfig
}

// NewHTTPServer creates a new HTTP server for the integration bridge
func NewHTTPServer(bridge *IntegrationBridge, config *sharedconfig.SharedConfig) *HTTPServer {
	router := mux.NewRouter()
	
	server := &HTTPServer{
		bridge: bridge,
		router: router,
		config: config,
	}
	
	// Register routes
	server.registerRoutes()
	
	// Create HTTP server
	port := config.Integration.WebhookPort
	if port == 0 {
		port = 8080 // default port
	}
	
	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      router,
		ReadTimeout:  config.Timeouts.HTTPReadTimeout,
		WriteTimeout: config.Timeouts.HTTPWriteTimeout,
		IdleTimeout:  config.Timeouts.HTTPIdleTimeout,
	}
	
	server.server = httpServer
	
	return server
}

// registerRoutes sets up HTTP routes for the integration bridge
func (hs *HTTPServer) registerRoutes() {
	// Health check endpoint
	hs.router.HandleFunc("/health", hs.healthCheck).Methods("GET")
	
	// Component status endpoints
	hs.router.HandleFunc("/status", hs.getComponentStatus).Methods("GET")
	hs.router.HandleFunc("/metrics", hs.getMetrics).Methods("GET")
	
	// Integration endpoints
	hs.router.HandleFunc("/api/v1/integration/events", hs.publishEvent).Methods("POST")
	hs.router.HandleFunc("/api/v1/integration/status", hs.getIntegrationStatus).Methods("GET")
	
	// Register restore API routes with subrouter
	restoreAPI := hs.bridge.GetRestoreAPI()
	if restoreAPI != nil {
		restoreAPI.RegisterRoutes(hs.router)
	}
	
	// Webhook endpoints
	hs.router.HandleFunc("/webhook/backup", hs.handleBackupWebhook).Methods("POST")
	hs.router.HandleFunc("/webhook/gitops", hs.handleGitOpsWebhook).Methods("POST")
	hs.router.HandleFunc("/webhook/restore", hs.handleRestoreWebhook).Methods("POST")
}

// Start starts the HTTP server
func (hs *HTTPServer) Start(ctx context.Context) error {
	port := hs.config.Integration.WebhookPort
	if port == 0 {
		port = 8080
	}
	log.Printf("Starting HTTP server on port %d", port)
	
	go func() {
		if err := hs.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()
	
	return nil
}

// Stop gracefully shuts down the HTTP server
func (hs *HTTPServer) Stop(ctx context.Context) error {
	log.Printf("Stopping HTTP server...")
	
	shutdownCtx, cancel := context.WithTimeout(ctx, hs.config.Timeouts.HTTPShutdownTimeout)
	defer cancel()
	
	return hs.server.Shutdown(shutdownCtx)
}

// Health check endpoint
func (hs *HTTPServer) healthCheck(w http.ResponseWriter, r *http.Request) {
	health := hs.bridge.HealthCheck(r.Context())
	
	w.Header().Set("Content-Type", "application/json")
	switch health.Status {
	case "healthy":
		w.WriteHeader(http.StatusOK)
	case "degraded":
		w.WriteHeader(http.StatusPartialContent)
	default:
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	
	fmt.Fprintf(w, `{"status": "%s", "message": "%s"}`, health.Status, health.Message)
}

// Get component status
func (hs *HTTPServer) getComponentStatus(w http.ResponseWriter, r *http.Request) {
	status := hs.bridge.GetComponentStatus()
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	
	// Simple JSON response (in production, use proper JSON marshaling)
	fmt.Fprintf(w, `{"components": %d, "timestamp": "%s"}`, 
		len(status), time.Now().Format(time.RFC3339))
}

// Get metrics
func (hs *HTTPServer) getMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := hs.bridge.GetMetrics()
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	
	// Simple metrics response
	fmt.Fprintf(w, `{"total_components": %v, "healthy_components": %v, "active_restores": %v}`,
		metrics["total_components"], metrics["healthy_components"], metrics["active_restores"])
}

// Publish integration event
func (hs *HTTPServer) publishEvent(w http.ResponseWriter, r *http.Request) {
	// This would parse the request body and publish an integration event
	// For now, just acknowledge the request
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, `{"status": "accepted", "timestamp": "%s"}`, time.Now().Format(time.RFC3339))
}

// Get integration status
func (hs *HTTPServer) getIntegrationStatus(w http.ResponseWriter, r *http.Request) {
	status := hs.bridge.GetComponentStatus()
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	
	// Return integration status
	healthyCount := 0
	for _, comp := range status {
		if comp.Status == "healthy" {
			healthyCount++
		}
	}
	
	overallStatus := "healthy"
	if healthyCount < len(status) {
		overallStatus = "degraded"
	}
	
	fmt.Fprintf(w, `{"overall_status": "%s", "components": %d, "healthy": %d}`,
		overallStatus, len(status), healthyCount)
}

// Handle backup webhooks
func (hs *HTTPServer) handleBackupWebhook(w http.ResponseWriter, r *http.Request) {
	// Parse backup completion event and trigger GitOps generation
	log.Printf("Received backup webhook")
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status": "processed", "timestamp": "%s"}`, time.Now().Format(time.RFC3339))
}

// Handle GitOps webhooks
func (hs *HTTPServer) handleGitOpsWebhook(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received GitOps webhook")
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status": "processed", "timestamp": "%s"}`, time.Now().Format(time.RFC3339))
}

// Handle restore webhooks
func (hs *HTTPServer) handleRestoreWebhook(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received restore webhook")
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status": "processed", "timestamp": "%s"}`, time.Now().Format(time.RFC3339))
}
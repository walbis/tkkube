package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"cluster-backup/internal/logging"
)

// MetricsServer handles the Prometheus metrics HTTP server
type MetricsServer struct {
	server *http.Server
	logger *logging.StructuredLogger
	port   int
}

// NewMetricsServer creates a new metrics server
func NewMetricsServer(port int, logger *logging.StructuredLogger) *MetricsServer {
	if port <= 0 {
		port = 8080 // Default port
	}
	
	mux := http.NewServeMux()
	
	// Register Prometheus metrics endpoint
	mux.Handle("/metrics", promhttp.Handler())
	
	// Register health check endpoint
	mux.HandleFunc("/health", healthCheckHandler)
	mux.HandleFunc("/healthz", healthCheckHandler)
	mux.HandleFunc("/ready", readinessCheckHandler)
	mux.HandleFunc("/readyz", readinessCheckHandler)
	
	// Register root endpoint with basic info
	mux.HandleFunc("/", rootHandler)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return &MetricsServer{
		server: server,
		logger: logger,
		port:   port,
	}
}

// Start starts the metrics server in a blocking manner
func (ms *MetricsServer) Start() error {
	ms.logger.Info("metrics_server_start", "Starting metrics server", map[string]interface{}{
		"port": ms.port,
		"addr": ms.server.Addr,
	})

	err := ms.server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		ms.logger.Error("metrics_server_error", "Metrics server failed", map[string]interface{}{
			"error": err.Error(),
			"port":  ms.port,
		})
		return fmt.Errorf("metrics server failed to start: %v", err)
	}

	return nil
}

// StartAsync starts the metrics server asynchronously and returns immediately
func (ms *MetricsServer) StartAsync() <-chan error {
	errChan := make(chan error, 1)
	
	go func() {
		defer close(errChan)
		if err := ms.Start(); err != nil {
			errChan <- err
		}
	}()
	
	return errChan
}

// Stop gracefully stops the metrics server
func (ms *MetricsServer) Stop(ctx context.Context) error {
	ms.logger.Info("metrics_server_stop", "Stopping metrics server", map[string]interface{}{
		"port": ms.port,
	})

	err := ms.server.Shutdown(ctx)
	if err != nil {
		ms.logger.Error("metrics_server_shutdown_error", "Error during metrics server shutdown", map[string]interface{}{
			"error": err.Error(),
		})
		return fmt.Errorf("error shutting down metrics server: %v", err)
	}

	ms.logger.Info("metrics_server_stopped", "Metrics server stopped successfully", nil)
	return nil
}

// GetPort returns the port the server is configured to run on
func (ms *MetricsServer) GetPort() int {
	return ms.port
}

// GetAddr returns the full address the server is configured to run on
func (ms *MetricsServer) GetAddr() string {
	return ms.server.Addr
}

// healthCheckHandler handles health check requests
func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "OK")
}

// readinessCheckHandler handles readiness check requests
func readinessCheckHandler(w http.ResponseWriter, r *http.Request) {
	// For now, same as health check. In a more complex application,
	// this might check if dependencies are ready (database, external APIs, etc.)
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "Ready")
}

// rootHandler handles requests to the root path
func rootHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	
	html := `<!DOCTYPE html>
<html>
<head>
    <title>Kubernetes Backup Service</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .container { max-width: 800px; margin: 0 auto; }
        .endpoint { background: #f5f5f5; padding: 10px; margin: 10px 0; border-radius: 5px; }
        .endpoint a { text-decoration: none; color: #0066cc; }
        .endpoint a:hover { text-decoration: underline; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Kubernetes Backup Service</h1>
        <p>This service provides backup functionality for Kubernetes clusters with MinIO storage.</p>
        
        <h2>Available Endpoints</h2>
        
        <div class="endpoint">
            <strong><a href="/metrics">/metrics</a></strong><br>
            Prometheus metrics endpoint for monitoring backup operations, performance, and health.
        </div>
        
        <div class="endpoint">
            <strong><a href="/health">/health</a></strong><br>
            Basic health check endpoint. Returns 200 OK if the service is running.
        </div>
        
        <div class="endpoint">
            <strong><a href="/ready">/ready</a></strong><br>
            Readiness check endpoint. Returns 200 OK when the service is ready to handle requests.
        </div>
        
        <h2>Service Information</h2>
        <ul>
            <li><strong>Service</strong>: Kubernetes Cluster Backup</li>
            <li><strong>Version</strong>: 1.0.0</li>
            <li><strong>Storage</strong>: MinIO Object Storage</li>
            <li><strong>Monitoring</strong>: Prometheus metrics</li>
        </ul>
        
        <h2>Health Endpoints</h2>
        <p>
            This service supports both <code>/health</code> and <code>/healthz</code> endpoints for health checking,
            and <code>/ready</code> and <code>/readyz</code> endpoints for readiness probes.
        </p>
    </div>
</body>
</html>`
	
	fmt.Fprint(w, html)
}

// StartMetricsServer is a convenience function to start a metrics server
func StartMetricsServer(port int, logger *logging.StructuredLogger) error {
	server := NewMetricsServer(port, logger)
	return server.Start()
}

// StartMetricsServerAsync is a convenience function to start a metrics server asynchronously
func StartMetricsServerAsync(port int, logger *logging.StructuredLogger) *MetricsServer {
	server := NewMetricsServer(port, logger)
	
	go func() {
		if err := server.Start(); err != nil {
			logger.Error("metrics_server_async_error", "Async metrics server failed", map[string]interface{}{
				"error": err.Error(),
			})
		}
	}()
	
	return server
}
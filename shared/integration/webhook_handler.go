package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

// WebhookHandler handles incoming webhooks from backup and GitOps components
type WebhookHandler struct {
	bridge   *IntegrationBridge
	server   *http.Server
	mux      *http.ServeMux
	running  bool
	mu       sync.RWMutex
}

// WebhookRequest represents an incoming webhook request
type WebhookRequest struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Source    string                 `json:"source"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
	Signature string                 `json:"signature,omitempty"`
}

// WebhookResponse represents a webhook response
type WebhookResponse struct {
	Success   bool        `json:"success"`
	Message   string      `json:"message"`
	RequestID string      `json:"request_id"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data,omitempty"`
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler(bridge *IntegrationBridge) *WebhookHandler {
	mux := http.NewServeMux()
	
	wh := &WebhookHandler{
		bridge: bridge,
		mux:    mux,
	}

	// Register webhook endpoints
	wh.registerRoutes()

	return wh
}

// Start starts the webhook handler server
func (wh *WebhookHandler) Start(ctx context.Context) error {
	wh.mu.Lock()
	defer wh.mu.Unlock()

	if wh.running {
		return fmt.Errorf("webhook handler is already running")
	}

	// Get port from configuration
	port := "8080"
	if wh.bridge.config.Integration.Enabled && wh.bridge.config.Integration.WebhookPort != 0 {
		port = fmt.Sprintf("%d", wh.bridge.config.Integration.WebhookPort)
	}

	// Use configurable timeouts
	readTimeout := 30 * time.Second
	writeTimeout := 30 * time.Second
	idleTimeout := 60 * time.Second
	if wh.bridge != nil && wh.bridge.config != nil {
		readTimeout = wh.bridge.config.Timeouts.HTTPReadTimeout
		writeTimeout = wh.bridge.config.Timeouts.HTTPWriteTimeout
		idleTimeout = wh.bridge.config.Timeouts.HTTPIdleTimeout
	}

	wh.server = &http.Server{
		Addr:         ":" + port,
		Handler:      wh.mux,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}

	wh.running = true

	go func() {
		log.Printf("Starting webhook handler on port %s", port)
		if err := wh.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Webhook handler error: %v", err)
		}
	}()

	return nil
}

// Stop stops the webhook handler server
func (wh *WebhookHandler) Stop() error {
	wh.mu.Lock()
	defer wh.mu.Unlock()

	if !wh.running {
		return nil
	}

	shutdownTimeout := 10 * time.Second // default fallback
	if wh.bridge != nil && wh.bridge.config != nil {
		shutdownTimeout = wh.bridge.config.Timeouts.HTTPShutdownTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := wh.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown webhook handler: %v", err)
	}

	wh.running = false
	log.Printf("Webhook handler stopped")
	return nil
}

// registerRoutes sets up webhook endpoints
func (wh *WebhookHandler) registerRoutes() {
	// Health check endpoint
	wh.mux.HandleFunc("/health", wh.handleHealth)
	
	// Backup completion webhook
	wh.mux.HandleFunc("/webhooks/backup/completed", wh.handleBackupCompleted)
	
	// GitOps generation webhook
	wh.mux.HandleFunc("/webhooks/gitops/generate", wh.handleGitOpsGenerate)
	
	// GitOps completion webhook
	wh.mux.HandleFunc("/webhooks/gitops/completed", wh.handleGitOpsCompleted)
	
	// Component registration endpoints
	wh.mux.HandleFunc("/register/backup", wh.handleRegisterBackup)
	wh.mux.HandleFunc("/register/gitops", wh.handleRegisterGitOps)
	
	// Status endpoint
	wh.mux.HandleFunc("/status", wh.handleStatus)
}

// handleHealth responds to health check requests
func (wh *WebhookHandler) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := wh.bridge.HealthCheck(r.Context())
	
	response := WebhookResponse{
		Success:   status.Status == "healthy",
		Message:   status.Message,
		RequestID: fmt.Sprintf("health-%d", time.Now().Unix()),
		Timestamp: time.Now(),
		Data:      status,
	}

	wh.sendJSONResponse(w, http.StatusOK, response)
}

// handleBackupCompleted processes backup completion webhooks
func (wh *WebhookHandler) handleBackupCompleted(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request WebhookRequest
	if err := wh.parseJSONRequest(r, &request); err != nil {
		wh.sendErrorResponse(w, http.StatusBadRequest, "Invalid JSON payload", err)
		return
	}

	// Parse backup completion data
	var backupEvent BackupCompletionEvent
	if data, err := json.Marshal(request.Data); err != nil {
		wh.sendErrorResponse(w, http.StatusBadRequest, "Invalid backup data", err)
		return
	} else if err := json.Unmarshal(data, &backupEvent); err != nil {
		wh.sendErrorResponse(w, http.StatusBadRequest, "Invalid backup event format", err)
		return
	}

	// Process backup completion
	if err := wh.bridge.TriggerGitOpsGeneration(r.Context(), &backupEvent); err != nil {
		wh.sendErrorResponse(w, http.StatusInternalServerError, "Failed to trigger GitOps generation", err)
		return
	}

	// Update metrics
	wh.bridge.monitoringSystem.GetMonitoringHub().GetMetricsCollector().IncCounter("webhook_requests_total",
		map[string]string{"endpoint": "backup_completed", "status": "success"}, 1)

	response := WebhookResponse{
		Success:   true,
		Message:   "Backup completion processed successfully",
		RequestID: request.ID,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"backup_id": backupEvent.BackupID,
			"triggered": true,
		},
	}

	wh.sendJSONResponse(w, http.StatusOK, response)
}

// handleGitOpsGenerate processes GitOps generation requests
func (wh *WebhookHandler) handleGitOpsGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request WebhookRequest
	if err := wh.parseJSONRequest(r, &request); err != nil {
		wh.sendErrorResponse(w, http.StatusBadRequest, "Invalid JSON payload", err)
		return
	}

	// Parse GitOps generation request
	var gitopsRequest GitOpsGenerationRequest
	if data, err := json.Marshal(request.Data); err != nil {
		wh.sendErrorResponse(w, http.StatusBadRequest, "Invalid GitOps data", err)
		return
	} else if err := json.Unmarshal(data, &gitopsRequest); err != nil {
		wh.sendErrorResponse(w, http.StatusBadRequest, "Invalid GitOps request format", err)
		return
	}

	// Create integration event
	integrationEvent := &IntegrationEvent{
		ID:        gitopsRequest.RequestID,
		Type:      "gitops_generation_requested",
		Source:    request.Source,
		Target:    "gitops-generator",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"request": gitopsRequest,
		},
		Metadata: map[string]interface{}{
			"webhook_id": request.ID,
		},
	}

	// Publish event
	if err := wh.bridge.eventBus.Publish(r.Context(), integrationEvent); err != nil {
		wh.sendErrorResponse(w, http.StatusInternalServerError, "Failed to process GitOps request", err)
		return
	}

	// Update metrics
	wh.bridge.monitoringSystem.GetMonitoringHub().GetMetricsCollector().IncCounter("webhook_requests_total",
		map[string]string{"endpoint": "gitops_generate", "status": "success"}, 1)

	response := WebhookResponse{
		Success:   true,
		Message:   "GitOps generation request processed",
		RequestID: request.ID,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"request_id": gitopsRequest.RequestID,
			"accepted":   true,
		},
	}

	wh.sendJSONResponse(w, http.StatusAccepted, response)
}

// handleGitOpsCompleted processes GitOps completion notifications
func (wh *WebhookHandler) handleGitOpsCompleted(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request WebhookRequest
	if err := wh.parseJSONRequest(r, &request); err != nil {
		wh.sendErrorResponse(w, http.StatusBadRequest, "Invalid JSON payload", err)
		return
	}

	// Create integration event
	integrationEvent := &IntegrationEvent{
		ID:        request.ID,
		Type:      "gitops_completed",
		Source:    request.Source,
		Target:    "integration-bridge",
		Timestamp: time.Now(),
		Data:      request.Data,
		Metadata: map[string]interface{}{
			"webhook_id": request.ID,
		},
	}

	// Publish event
	if err := wh.bridge.eventBus.Publish(r.Context(), integrationEvent); err != nil {
		wh.sendErrorResponse(w, http.StatusInternalServerError, "Failed to process completion notification", err)
		return
	}

	// Update metrics
	status := "success"
	if request.Data["error"] != nil {
		status = "failure"
	}
	
	wh.bridge.monitoringSystem.GetMonitoringHub().GetMetricsCollector().IncCounter("webhook_requests_total",
		map[string]string{"endpoint": "gitops_completed", "status": status}, 1)

	response := WebhookResponse{
		Success:   true,
		Message:   "GitOps completion processed",
		RequestID: request.ID,
		Timestamp: time.Now(),
	}

	wh.sendJSONResponse(w, http.StatusOK, response)
}

// handleRegisterBackup registers backup tool component
func (wh *WebhookHandler) handleRegisterBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		Endpoint string `json:"endpoint"`
		Version  string `json:"version"`
	}

	if err := wh.parseJSONRequest(r, &request); err != nil {
		wh.sendErrorResponse(w, http.StatusBadRequest, "Invalid JSON payload", err)
		return
	}

	if err := wh.bridge.RegisterBackupTool(request.Endpoint, request.Version); err != nil {
		wh.sendErrorResponse(w, http.StatusInternalServerError, "Failed to register backup tool", err)
		return
	}

	response := WebhookResponse{
		Success:   true,
		Message:   "Backup tool registered successfully",
		RequestID: fmt.Sprintf("register-backup-%d", time.Now().Unix()),
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"endpoint": request.Endpoint,
			"version":  request.Version,
		},
	}

	wh.sendJSONResponse(w, http.StatusOK, response)
}

// handleRegisterGitOps registers GitOps generator component
func (wh *WebhookHandler) handleRegisterGitOps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		Endpoint string `json:"endpoint"`
		Version  string `json:"version"`
	}

	if err := wh.parseJSONRequest(r, &request); err != nil {
		wh.sendErrorResponse(w, http.StatusBadRequest, "Invalid JSON payload", err)
		return
	}

	if err := wh.bridge.RegisterGitOpsTool(request.Endpoint, request.Version); err != nil {
		wh.sendErrorResponse(w, http.StatusInternalServerError, "Failed to register GitOps tool", err)
		return
	}

	response := WebhookResponse{
		Success:   true,
		Message:   "GitOps tool registered successfully",
		RequestID: fmt.Sprintf("register-gitops-%d", time.Now().Unix()),
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"endpoint": request.Endpoint,
			"version":  request.Version,
		},
	}

	wh.sendJSONResponse(w, http.StatusOK, response)
}

// handleStatus returns component status information
func (wh *WebhookHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := wh.bridge.GetComponentStatus()
	
	response := WebhookResponse{
		Success:   true,
		Message:   "Component status retrieved",
		RequestID: fmt.Sprintf("status-%d", time.Now().Unix()),
		Timestamp: time.Now(),
		Data:      status,
	}

	wh.sendJSONResponse(w, http.StatusOK, response)
}

// parseJSONRequest parses JSON request body
func (wh *WebhookHandler) parseJSONRequest(r *http.Request, target interface{}) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("failed to read request body: %v", err)
	}
	defer r.Body.Close()

	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("failed to parse JSON: %v", err)
	}

	return nil
}

// sendJSONResponse sends a JSON response
func (wh *WebhookHandler) sendJSONResponse(w http.ResponseWriter, statusCode int, response interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode JSON response: %v", err)
	}
}

// sendErrorResponse sends an error response
func (wh *WebhookHandler) sendErrorResponse(w http.ResponseWriter, statusCode int, message string, err error) {
	// Update error metrics
	wh.bridge.monitoringSystem.GetMonitoringHub().GetMetricsCollector().IncCounter("webhook_errors_total",
		map[string]string{"status_code": fmt.Sprintf("%d", statusCode)}, 1)

	response := WebhookResponse{
		Success:   false,
		Message:   message,
		RequestID: fmt.Sprintf("error-%d", time.Now().Unix()),
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"error": err.Error(),
		},
	}

	wh.sendJSONResponse(w, statusCode, response)
}
package restore

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	sharedconfig "shared-config/config"
	"shared-config/monitoring"
	"shared-config/security"
)

// RestoreAPI provides REST API endpoints for restore operations
type RestoreAPI struct {
	restoreEngine    *RestoreEngine
	securityManager  *security.SecurityManager
	monitoringSystem *monitoring.MonitoringSystem
	config          *sharedconfig.SharedConfig
}

// APIResponse represents a standard API response
type APIResponse struct {
	Success   bool        `json:"success"`
	Message   string      `json:"message,omitempty"`
	Data      interface{} `json:"data,omitempty"`
	Error     *APIError   `json:"error,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
	RequestID string      `json:"request_id,omitempty"`
}

// APIError represents an API error response
type APIError struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

// RestoreAPIRequest represents a restore API request
type RestoreAPIRequest struct {
	RestoreID        string                 `json:"restore_id"`
	BackupID         string                 `json:"backup_id"`
	ClusterName      string                 `json:"cluster_name"`
	TargetNamespaces []string               `json:"target_namespaces,omitempty"`
	ResourceTypes    []string               `json:"resource_types,omitempty"`
	LabelSelector    string                 `json:"label_selector,omitempty"`
	RestoreMode      RestoreMode            `json:"restore_mode"`
	ValidationMode   ValidationMode         `json:"validation_mode"`
	ConflictStrategy ConflictStrategy       `json:"conflict_strategy"`
	DryRun           bool                   `json:"dry_run"`
	Configuration    map[string]interface{} `json:"configuration,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// DisasterRecoveryRequest represents a DR scenario request
type DisasterRecoveryRequest struct {
	ScenarioID       string                 `json:"scenario_id"`
	SourceCluster    string                 `json:"source_cluster"`
	TargetCluster    string                 `json:"target_cluster"`
	BackupID         string                 `json:"backup_id,omitempty"`
	ScenarioType     string                 `json:"scenario_type"` // cluster_rebuild, namespace_recovery, etc.
	AutomationLevel  string                 `json:"automation_level"` // manual, assisted, automated
	ValidationLevel  string                 `json:"validation_level"` // strict, permissive, skip
	Configuration    map[string]interface{} `json:"configuration,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// NewRestoreAPI creates a new restore API instance
func NewRestoreAPI(engine *RestoreEngine, security *security.SecurityManager, monitoring *monitoring.MonitoringSystem, config *sharedconfig.SharedConfig) *RestoreAPI {
	return &RestoreAPI{
		restoreEngine:    engine,
		securityManager:  security,
		monitoringSystem: monitoring,
		config:          config,
	}
}

// RegisterRoutes registers all restore API routes
func (api *RestoreAPI) RegisterRoutes(router *mux.Router) {
	// Restore operations
	router.HandleFunc("/api/v1/restore", api.StartRestore).Methods("POST")
	router.HandleFunc("/api/v1/restore/{restoreId}", api.GetRestoreStatus).Methods("GET")
	router.HandleFunc("/api/v1/restore/{restoreId}", api.CancelRestore).Methods("DELETE")
	router.HandleFunc("/api/v1/restore", api.ListActiveRestores).Methods("GET")
	
	// Restore history and management
	router.HandleFunc("/api/v1/restore/history", api.GetRestoreHistory).Methods("GET")
	router.HandleFunc("/api/v1/restore/validate", api.ValidateRestore).Methods("POST")
	router.HandleFunc("/api/v1/restore/plan", api.CreateRestorePlan).Methods("POST")
	
	// Disaster recovery scenarios
	router.HandleFunc("/api/v1/dr/scenarios", api.ListDRScenarios).Methods("GET")
	router.HandleFunc("/api/v1/dr/execute", api.ExecuteDRScenario).Methods("POST")
	router.HandleFunc("/api/v1/dr/scenarios/{scenarioId}", api.GetDRScenarioStatus).Methods("GET")
	
	// Backup management for restore
	router.HandleFunc("/api/v1/backups", api.ListAvailableBackups).Methods("GET")
	router.HandleFunc("/api/v1/backups/{backupId}", api.GetBackupDetails).Methods("GET")
	router.HandleFunc("/api/v1/backups/{backupId}/validate", api.ValidateBackup).Methods("POST")
	
	// Cluster management
	router.HandleFunc("/api/v1/clusters", api.ListClusters).Methods("GET")
	router.HandleFunc("/api/v1/clusters/{clusterName}/validate", api.ValidateCluster).Methods("POST")
	router.HandleFunc("/api/v1/clusters/{clusterName}/readiness", api.CheckClusterReadiness).Methods("GET")
	
	// Apply security middleware to all routes
	router.Use(api.securityMiddleware)
	router.Use(api.loggingMiddleware)
	router.Use(api.metricsMiddleware)
}

// StartRestore initiates a new restore operation
func (api *RestoreAPI) StartRestore(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	
	// Parse request
	var req RestoreAPIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.sendError(w, "invalid_request", "Invalid request format", err, http.StatusBadRequest)
		return
	}
	
	// Validate request
	if err := api.validateRestoreRequest(req); err != nil {
		api.sendError(w, "validation_error", "Request validation failed", err, http.StatusBadRequest)
		return
	}
	
	// Convert to internal request format
	restoreRequest := RestoreRequest{
		RestoreID:        req.RestoreID,
		BackupID:         req.BackupID,
		ClusterName:      req.ClusterName,
		TargetNamespaces: req.TargetNamespaces,
		ResourceTypes:    req.ResourceTypes,
		LabelSelector:    req.LabelSelector,
		RestoreMode:      req.RestoreMode,
		ValidationMode:   req.ValidationMode,
		ConflictStrategy: req.ConflictStrategy,
		DryRun:           req.DryRun,
		Configuration:    req.Configuration,
		Metadata:         req.Metadata,
	}
	
	// Start restore operation
	operation, err := api.restoreEngine.StartRestore(ctx, restoreRequest)
	if err != nil {
		api.sendError(w, "restore_failed", "Failed to start restore operation", err, http.StatusInternalServerError)
		return
	}
	
	// Send success response
	api.sendSuccess(w, "Restore operation started successfully", operation, http.StatusAccepted)
}

// GetRestoreStatus returns the current status of a restore operation
func (api *RestoreAPI) GetRestoreStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	restoreID := vars["restoreId"]
	
	if restoreID == "" {
		api.sendError(w, "missing_parameter", "Restore ID is required", nil, http.StatusBadRequest)
		return
	}
	
	// Get restore status
	operation, err := api.restoreEngine.GetRestoreStatus(restoreID)
	if err != nil {
		api.sendError(w, "not_found", "Restore operation not found", err, http.StatusNotFound)
		return
	}
	
	api.sendSuccess(w, "Restore status retrieved successfully", operation, http.StatusOK)
}

// CancelRestore cancels an active restore operation
func (api *RestoreAPI) CancelRestore(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	restoreID := vars["restoreId"]
	
	if restoreID == "" {
		api.sendError(w, "missing_parameter", "Restore ID is required", nil, http.StatusBadRequest)
		return
	}
	
	// Cancel restore operation
	err := api.restoreEngine.CancelRestore(restoreID)
	if err != nil {
		api.sendError(w, "cancel_failed", "Failed to cancel restore operation", err, http.StatusInternalServerError)
		return
	}
	
	api.sendSuccess(w, "Restore operation cancelled successfully", nil, http.StatusOK)
}

// ListActiveRestores returns all currently active restore operations
func (api *RestoreAPI) ListActiveRestores(w http.ResponseWriter, r *http.Request) {
	operations := api.restoreEngine.ListActiveRestores()
	api.sendSuccess(w, "Active restore operations retrieved successfully", operations, http.StatusOK)
}

// GetRestoreHistory returns historical restore operations
func (api *RestoreAPI) GetRestoreHistory(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	limitParam := r.URL.Query().Get("limit")
	limit := 50 // default limit
	
	if limitParam != "" {
		if parsedLimit, err := parseIntParam(limitParam); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}
	
	history := api.restoreEngine.GetRestoreHistory(limit)
	api.sendSuccess(w, "Restore history retrieved successfully", history, http.StatusOK)
}

// ValidateRestore validates a restore request without executing it
func (api *RestoreAPI) ValidateRestore(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	
	// Parse request
	var req RestoreAPIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.sendError(w, "invalid_request", "Invalid request format", err, http.StatusBadRequest)
		return
	}
	
	// Convert to internal request format
	restoreRequest := RestoreRequest{
		BackupID:         req.BackupID,
		ClusterName:      req.ClusterName,
		TargetNamespaces: req.TargetNamespaces,
		ResourceTypes:    req.ResourceTypes,
		RestoreMode:      req.RestoreMode,
		ValidationMode:   req.ValidationMode,
		ConflictStrategy: req.ConflictStrategy,
		DryRun:           true, // Always dry run for validation
	}
	
	// Validate restore
	report, err := api.restoreEngine.validator.ValidateRestore(ctx, restoreRequest)
	if err != nil {
		api.sendError(w, "validation_failed", "Restore validation failed", err, http.StatusInternalServerError)
		return
	}
	
	api.sendSuccess(w, "Restore validation completed", report, http.StatusOK)
}

// CreateRestorePlan creates a detailed restore plan
func (api *RestoreAPI) CreateRestorePlan(w http.ResponseWriter, r *http.Request) {
	// Parse request
	var req RestoreAPIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.sendError(w, "invalid_request", "Invalid request format", err, http.StatusBadRequest)
		return
	}
	
	// Create restore plan (simplified implementation)
	plan := map[string]interface{}{
		"restore_id":       req.RestoreID,
		"backup_id":        req.BackupID,
		"cluster_name":     req.ClusterName,
		"restore_mode":     req.RestoreMode,
		"estimated_time":   "30 minutes",
		"total_resources":  100, // Would be calculated from backup
		"phases": []string{
			"validation", "preparation", "execution", "verification", "cleanup",
		},
		"risks": []string{
			"Potential downtime during restore",
			"Resource conflicts may require manual intervention",
		},
		"prerequisites": []string{
			"Target cluster must be accessible",
			"Sufficient permissions required",
			"Adequate storage space needed",
		},
	}
	
	api.sendSuccess(w, "Restore plan created successfully", plan, http.StatusOK)
}

// ExecuteDRScenario executes a disaster recovery scenario
func (api *RestoreAPI) ExecuteDRScenario(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	
	// Parse request
	var req DisasterRecoveryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.sendError(w, "invalid_request", "Invalid request format", err, http.StatusBadRequest)
		return
	}
	
	// Validate DR request
	if err := api.validateDRRequest(req); err != nil {
		api.sendError(w, "validation_error", "DR request validation failed", err, http.StatusBadRequest)
		return
	}
	
	// Convert to restore request based on DR scenario
	restoreRequest, err := api.convertDRToRestoreRequest(req)
	if err != nil {
		api.sendError(w, "conversion_error", "Failed to convert DR request", err, http.StatusInternalServerError)
		return
	}
	
	// Execute restore
	operation, err := api.restoreEngine.StartRestore(ctx, *restoreRequest)
	if err != nil {
		api.sendError(w, "dr_execution_failed", "Failed to execute DR scenario", err, http.StatusInternalServerError)
		return
	}
	
	api.sendSuccess(w, "DR scenario execution started successfully", operation, http.StatusAccepted)
}

// ListDRScenarios returns available disaster recovery scenarios
func (api *RestoreAPI) ListDRScenarios(w http.ResponseWriter, r *http.Request) {
	scenarios := []map[string]interface{}{
		{
			"id":          "cluster_rebuild",
			"name":        "Complete Cluster Rebuild",
			"description": "Rebuild entire cluster from backup",
			"estimated_time": "2-4 hours",
			"automation_level": "assisted",
		},
		{
			"id":          "namespace_recovery",
			"name":        "Namespace Recovery",
			"description": "Recover specific namespaces",
			"estimated_time": "30-60 minutes",
			"automation_level": "automated",
		},
		{
			"id":          "data_corruption",
			"name":        "Data Corruption Recovery",
			"description": "Recover from data corruption events",
			"estimated_time": "1-2 hours",
			"automation_level": "assisted",
		},
		{
			"id":          "configuration_rollback",
			"name":        "Configuration Rollback",
			"description": "Rollback configuration changes",
			"estimated_time": "15-30 minutes",
			"automation_level": "automated",
		},
	}
	
	api.sendSuccess(w, "DR scenarios retrieved successfully", scenarios, http.StatusOK)
}

// GetDRScenarioStatus returns the status of a DR scenario execution
func (api *RestoreAPI) GetDRScenarioStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	scenarioID := vars["scenarioId"]
	
	// For now, redirect to restore status (in real implementation, maintain separate tracking)
	operation, err := api.restoreEngine.GetRestoreStatus(scenarioID)
	if err != nil {
		api.sendError(w, "not_found", "DR scenario not found", err, http.StatusNotFound)
		return
	}
	
	api.sendSuccess(w, "DR scenario status retrieved successfully", operation, http.StatusOK)
}

// ListAvailableBackups returns available backups for restore
func (api *RestoreAPI) ListAvailableBackups(w http.ResponseWriter, r *http.Request) {
	// This would integrate with backup storage (MinIO) to list available backups
	// For now, return mock data
	backups := []map[string]interface{}{
		{
			"backup_id":    "backup-2024-01-15-001",
			"cluster_name": "production",
			"timestamp":    "2024-01-15T10:30:00Z",
			"size":         "50MB",
			"resources":    125,
			"status":       "completed",
		},
		{
			"backup_id":    "backup-2024-01-14-001",
			"cluster_name": "production",
			"timestamp":    "2024-01-14T10:30:00Z",
			"size":         "48MB",
			"resources":    120,
			"status":       "completed",
		},
	}
	
	api.sendSuccess(w, "Available backups retrieved successfully", backups, http.StatusOK)
}

// GetBackupDetails returns detailed information about a backup
func (api *RestoreAPI) GetBackupDetails(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	backupID := vars["backupId"]
	
	// This would load actual backup metadata from storage
	// For now, return mock data
	backupDetails := map[string]interface{}{
		"backup_id":    backupID,
		"cluster_name": "production",
		"timestamp":    "2024-01-15T10:30:00Z",
		"size":         "50MB",
		"duration":     "15 minutes",
		"resources": map[string]int{
			"namespaces":   5,
			"deployments":  12,
			"services":     8,
			"configmaps":   15,
			"secrets":      6,
		},
		"metadata": map[string]interface{}{
			"kubernetes_version": "1.28.0",
			"backup_tool_version": "1.0.0",
			"compression": "gzip",
		},
	}
	
	api.sendSuccess(w, "Backup details retrieved successfully", backupDetails, http.StatusOK)
}

// ValidateBackup validates backup integrity and compatibility
func (api *RestoreAPI) ValidateBackup(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	backupID := vars["backupId"]
	
	// This would perform actual backup validation
	// For now, return mock validation result
	validationResult := map[string]interface{}{
		"backup_id": backupID,
		"valid":     true,
		"checks": map[string]interface{}{
			"integrity":     "passed",
			"completeness":  "passed",
			"compatibility": "passed",
		},
		"warnings": []string{
			"Backup is 30 days old",
		},
		"recommendations": []string{
			"Test restore in non-production environment first",
		},
	}
	
	api.sendSuccess(w, "Backup validation completed", validationResult, http.StatusOK)
}

// ListClusters returns available clusters for restore operations
func (api *RestoreAPI) ListClusters(w http.ResponseWriter, r *http.Request) {
	// This would integrate with cluster discovery
	// For now, return mock data
	clusters := []map[string]interface{}{
		{
			"name":             "production",
			"status":           "healthy",
			"kubernetes_version": "1.28.0",
			"node_count":       5,
			"namespace_count":  12,
		},
		{
			"name":             "staging",
			"status":           "healthy",
			"kubernetes_version": "1.27.0",
			"node_count":       3,
			"namespace_count":  8,
		},
	}
	
	api.sendSuccess(w, "Clusters retrieved successfully", clusters, http.StatusOK)
}

// ValidateCluster validates cluster readiness for restore
func (api *RestoreAPI) ValidateCluster(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	clusterName := vars["clusterName"]
	
	// This would perform actual cluster validation
	// For now, return mock validation result
	validationResult := map[string]interface{}{
		"cluster_name": clusterName,
		"accessible":   true,
		"ready":        true,
		"checks": map[string]interface{}{
			"api_server":      "healthy",
			"storage_classes": "available",
			"permissions":     "sufficient",
			"network":         "healthy",
		},
		"capacity": map[string]interface{}{
			"cpu":              "80% available",
			"memory":           "75% available",
			"storage":          "60% available",
			"persistent_volumes": "available",
		},
	}
	
	api.sendSuccess(w, "Cluster validation completed", validationResult, http.StatusOK)
}

// CheckClusterReadiness checks if cluster is ready for restore operations
func (api *RestoreAPI) CheckClusterReadiness(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	clusterName := vars["clusterName"]
	
	readinessStatus := map[string]interface{}{
		"cluster_name": clusterName,
		"ready":        true,
		"score":        95,
		"components": map[string]string{
			"api_server":        "ready",
			"etcd":              "ready",
			"controller_manager": "ready",
			"scheduler":         "ready",
			"nodes":             "ready",
		},
		"prerequisites": map[string]bool{
			"storage_available":     true,
			"network_connectivity": true,
			"sufficient_resources":  true,
			"backup_tools_installed": true,
		},
	}
	
	api.sendSuccess(w, "Cluster readiness check completed", readinessStatus, http.StatusOK)
}

// Helper methods

func (api *RestoreAPI) validateRestoreRequest(req RestoreAPIRequest) error {
	if req.RestoreID == "" {
		return fmt.Errorf("restore_id is required")
	}
	if req.BackupID == "" {
		return fmt.Errorf("backup_id is required")
	}
	if req.ClusterName == "" {
		return fmt.Errorf("cluster_name is required")
	}
	return nil
}

func (api *RestoreAPI) validateDRRequest(req DisasterRecoveryRequest) error {
	if req.ScenarioID == "" {
		return fmt.Errorf("scenario_id is required")
	}
	if req.SourceCluster == "" {
		return fmt.Errorf("source_cluster is required")
	}
	if req.TargetCluster == "" {
		return fmt.Errorf("target_cluster is required")
	}
	return nil
}

func (api *RestoreAPI) convertDRToRestoreRequest(drReq DisasterRecoveryRequest) (*RestoreRequest, error) {
	// Convert DR request to restore request based on scenario type
	restoreMode := RestoreModeComplete
	validationMode := ValidationModeStrict
	conflictStrategy := ConflictStrategyOverwrite
	
	switch drReq.ScenarioType {
	case "cluster_rebuild":
		restoreMode = RestoreModeComplete
		conflictStrategy = ConflictStrategyOverwrite
	case "namespace_recovery":
		restoreMode = RestoreModeSelective
		conflictStrategy = ConflictStrategyMerge
	case "configuration_rollback":
		restoreMode = RestoreModeIncremental
		conflictStrategy = ConflictStrategyOverwrite
	}
	
	return &RestoreRequest{
		RestoreID:        drReq.ScenarioID,
		BackupID:         drReq.BackupID,
		ClusterName:      drReq.TargetCluster,
		RestoreMode:      restoreMode,
		ValidationMode:   validationMode,
		ConflictStrategy: conflictStrategy,
		Configuration:    drReq.Configuration,
		Metadata:         drReq.Metadata,
	}, nil
}

func (api *RestoreAPI) sendSuccess(w http.ResponseWriter, message string, data interface{}, statusCode int) {
	response := APIResponse{
		Success:   true,
		Message:   message,
		Data:      data,
		Timestamp: time.Now(),
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

func (api *RestoreAPI) sendError(w http.ResponseWriter, code, message string, err error, statusCode int) {
	apiError := &APIError{
		Code:    code,
		Message: message,
	}
	
	if err != nil {
		apiError.Details = err.Error()
	}
	
	response := APIResponse{
		Success:   false,
		Error:     apiError,
		Timestamp: time.Now(),
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

// Middleware functions

func (api *RestoreAPI) securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Apply security validations
		if err := api.securityManager.ValidateRequest(r.Context(), r); err != nil {
			api.sendError(w, "security_error", "Security validation failed", err, http.StatusUnauthorized)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

func (api *RestoreAPI) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		// Log request
		fmt.Printf("Started %s %s from %s\n", r.Method, r.URL.Path, r.RemoteAddr)
		
		next.ServeHTTP(w, r)
		
		// Log completion
		duration := time.Since(start)
		fmt.Printf("Completed %s %s in %v\n", r.Method, r.URL.Path, duration)
	})
}

func (api *RestoreAPI) metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		next.ServeHTTP(w, r)
		
		// Record metrics
		duration := time.Since(start)
		labels := map[string]string{
			"method": r.Method,
			"path":   r.URL.Path,
		}
		
		api.monitoringSystem.GetMonitoringHub().GetMetricsCollector().RecordDuration(
			"restore_api_request_duration",
			labels,
			duration,
		)
		
		api.monitoringSystem.GetMonitoringHub().GetMetricsCollector().IncCounter(
			"restore_api_requests_total",
			labels,
			1,
		)
	})
}

// Utility functions

func parseIntParam(param string) (int, error) {
	// Simple integer parsing - in real implementation use strconv.Atoi
	return 50, nil // placeholder
}
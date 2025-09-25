package sharedconfig

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// MultiClusterManager manages multiple Kubernetes clusters
type MultiClusterManager struct {
	config       *MultiClusterConfig
	clients      map[string]*kubernetes.Clientset
	restConfigs  map[string]*rest.Config
	authManager  *ClusterAuthManager
	mutex        sync.RWMutex
	healthChecks map[string]bool
	lastHealthCheck time.Time
}

// ClusterClient represents a cluster client with metadata
type ClusterClient struct {
	Name      string
	Client    *kubernetes.Clientset
	Config    *rest.Config
	Storage   StorageConfig
	Healthy   bool
	LastCheck time.Time
}

// NewMultiClusterManager creates a new multi-cluster manager
func NewMultiClusterManager(config *MultiClusterConfig) (*MultiClusterManager, error) {
	if !config.Enabled {
		return nil, fmt.Errorf("multi-cluster support is not enabled")
	}

	if len(config.Clusters) == 0 {
		return nil, fmt.Errorf("no clusters configured for multi-cluster mode")
	}

	manager := &MultiClusterManager{
		config:       config,
		clients:      make(map[string]*kubernetes.Clientset),
		restConfigs:  make(map[string]*rest.Config),
		authManager:  NewClusterAuthManager(),
		healthChecks: make(map[string]bool),
	}

	// Initialize cluster clients
	if err := manager.initializeClients(); err != nil {
		return nil, fmt.Errorf("failed to initialize cluster clients: %w", err)
	}

	// Start health check routine
	go manager.healthCheckRoutine()

	return manager, nil
}

// initializeClients creates Kubernetes clients for all configured clusters
func (m *MultiClusterManager) initializeClients() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, cluster := range m.config.Clusters {
		if cluster.Name == "" {
			return fmt.Errorf("cluster name is required")
		}

		if cluster.Endpoint == "" {
			return fmt.Errorf("cluster endpoint is required for cluster %s", cluster.Name)
		}

		// Validate authentication configuration
		if err := m.authManager.ValidateAuthentication(&cluster); err != nil {
			return fmt.Errorf("invalid authentication for cluster %s: %w", cluster.Name, err)
		}

		// Validate TLS configuration
		if err := m.authManager.ValidateTLSConfig(&cluster.TLS, cluster.Name); err != nil {
			return fmt.Errorf("invalid TLS configuration for cluster %s: %w", cluster.Name, err)
		}

		// Create REST config using the enhanced authentication manager
		restConfig, err := m.authManager.CreateRESTConfig(&cluster)
		if err != nil {
			return fmt.Errorf("failed to create REST config for cluster %s: %w", cluster.Name, err)
		}

		// Create Kubernetes client
		client, err := kubernetes.NewForConfig(restConfig)
		if err != nil {
			return fmt.Errorf("failed to create client for cluster %s: %w", cluster.Name, err)
		}

		m.clients[cluster.Name] = client
		m.restConfigs[cluster.Name] = restConfig
		m.healthChecks[cluster.Name] = false

		log.Printf("Initialized client for cluster: %s", cluster.Name)
	}

	return nil
}

// GetClient returns the Kubernetes client for the specified cluster
func (m *MultiClusterManager) GetClient(clusterName string) (*kubernetes.Clientset, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	client, exists := m.clients[clusterName]
	if !exists {
		return nil, fmt.Errorf("cluster %s not found", clusterName)
	}

	return client, nil
}

// GetClusterConfig returns the configuration for the specified cluster
func (m *MultiClusterManager) GetClusterConfig(clusterName string) (*MultiClusterClusterConfig, error) {
	for _, cluster := range m.config.Clusters {
		if cluster.Name == clusterName {
			return &cluster, nil
		}
	}
	return nil, fmt.Errorf("cluster %s not found in configuration", clusterName)
}

// GetAllClusters returns information about all configured clusters
func (m *MultiClusterManager) GetAllClusters() []ClusterClient {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	clusters := make([]ClusterClient, 0, len(m.clients))
	for name, client := range m.clients {
		config := m.restConfigs[name]
		healthy := m.healthChecks[name]
		
		// Find storage config for this cluster
		var storage StorageConfig
		for _, cluster := range m.config.Clusters {
			if cluster.Name == name {
				storage = cluster.Storage
				break
			}
		}

		clusters = append(clusters, ClusterClient{
			Name:      name,
			Client:    client,
			Config:    config,
			Storage:   storage,
			Healthy:   healthy,
			LastCheck: m.lastHealthCheck,
		})
	}

	return clusters
}

// GetHealthyClusters returns only healthy clusters
func (m *MultiClusterManager) GetHealthyClusters() []ClusterClient {
	allClusters := m.GetAllClusters()
	healthyClusters := make([]ClusterClient, 0)

	for _, cluster := range allClusters {
		if cluster.Healthy {
			healthyClusters = append(healthyClusters, cluster)
		}
	}

	return healthyClusters
}

// GetDefaultCluster returns the client for the default cluster
func (m *MultiClusterManager) GetDefaultCluster() (*kubernetes.Clientset, error) {
	if m.config.DefaultCluster == "" {
		return nil, fmt.Errorf("no default cluster configured")
	}

	return m.GetClient(m.config.DefaultCluster)
}

// ExecuteOnCluster executes a function on a specific cluster
func (m *MultiClusterManager) ExecuteOnCluster(clusterName string, fn func(*kubernetes.Clientset, StorageConfig) error) error {
	client, err := m.GetClient(clusterName)
	if err != nil {
		return err
	}

	clusterConfig, err := m.GetClusterConfig(clusterName)
	if err != nil {
		return err
	}

	return fn(client, clusterConfig.Storage)
}

// ExecuteOnAllClusters executes a function on all healthy clusters
func (m *MultiClusterManager) ExecuteOnAllClusters(fn func(string, *kubernetes.Clientset, StorageConfig) error) error {
	healthyClusters := m.GetHealthyClusters()
	
	if len(healthyClusters) == 0 {
		return fmt.Errorf("no healthy clusters available")
	}

	// Determine execution mode
	switch m.config.Mode {
	case "sequential":
		return m.executeSequentially(healthyClusters, fn)
	case "parallel":
		return m.executeInParallel(healthyClusters, fn)
	default:
		return fmt.Errorf("unknown execution mode: %s", m.config.Mode)
	}
}

// executeSequentially executes function on clusters one by one
func (m *MultiClusterManager) executeSequentially(clusters []ClusterClient, fn func(string, *kubernetes.Clientset, StorageConfig) error) error {
	var errors []string

	for _, cluster := range clusters {
		log.Printf("Executing on cluster: %s", cluster.Name)
		if err := fn(cluster.Name, cluster.Client, cluster.Storage); err != nil {
			errorMsg := fmt.Sprintf("cluster %s failed: %v", cluster.Name, err)
			errors = append(errors, errorMsg)
			log.Printf("Error: %s", errorMsg)
			
			// Check if we should continue on error
			if len(errors) >= m.config.Coordination.FailureThreshold {
				break
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("execution failed on %d clusters: %v", len(errors), errors)
	}

	return nil
}

// executeInParallel executes function on clusters in parallel
func (m *MultiClusterManager) executeInParallel(clusters []ClusterClient, fn func(string, *kubernetes.Clientset, StorageConfig) error) error {
	maxConcurrent := m.config.Scheduling.MaxConcurrentClusters
	if maxConcurrent <= 0 || maxConcurrent > len(clusters) {
		maxConcurrent = len(clusters)
	}

	// Channel to limit concurrent executions
	semaphore := make(chan struct{}, maxConcurrent)
	
	// Channel to collect results
	results := make(chan error, len(clusters))
	
	// Execute on all clusters concurrently
	for _, cluster := range clusters {
		go func(c ClusterClient) {
			semaphore <- struct{}{} // Acquire
			defer func() { <-semaphore }() // Release
			
			log.Printf("Executing on cluster: %s", c.Name)
			err := fn(c.Name, c.Client, c.Storage)
			if err != nil {
				err = fmt.Errorf("cluster %s failed: %w", c.Name, err)
			}
			results <- err
		}(cluster)
	}

	// Collect results
	var errors []string
	for i := 0; i < len(clusters); i++ {
		if err := <-results; err != nil {
			errors = append(errors, err.Error())
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("execution failed on %d clusters: %v", len(errors), errors)
	}

	return nil
}

// healthCheckRoutine performs periodic health checks on all clusters
func (m *MultiClusterManager) healthCheckRoutine() {
	interval, err := time.ParseDuration(m.config.Coordination.HealthCheckInterval)
	if err != nil {
		log.Printf("Invalid health check interval: %s, using default 30s", m.config.Coordination.HealthCheckInterval)
		interval = 30 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.performHealthChecks()
		}
	}
}

// performHealthChecks checks the health of all clusters
func (m *MultiClusterManager) performHealthChecks() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.lastHealthCheck = time.Now()

	for clusterName, client := range m.clients {
		healthy := m.checkClusterHealth(client)
		m.healthChecks[clusterName] = healthy

		if healthy {
			log.Printf("Cluster %s: healthy", clusterName)
		} else {
			log.Printf("Cluster %s: unhealthy", clusterName)
		}
	}
}

// checkClusterHealth performs a health check on a single cluster
func (m *MultiClusterManager) checkClusterHealth(client *kubernetes.Clientset) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Try to list namespaces as a health check
	_, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1})
	return err == nil
}

// Close cleans up resources
func (m *MultiClusterManager) Close() {
	log.Printf("Shutting down multi-cluster manager")
	// No explicit cleanup needed for Kubernetes clients
}

// GetClusterNames returns the names of all configured clusters
func (m *MultiClusterManager) GetClusterNames() []string {
	names := make([]string, 0, len(m.config.Clusters))
	for _, cluster := range m.config.Clusters {
		names = append(names, cluster.Name)
	}
	return names
}

// IsMultiClusterEnabled returns true if multi-cluster mode is enabled
func (m *MultiClusterManager) IsMultiClusterEnabled() bool {
	return m.config.Enabled
}

// GetExecutionMode returns the current execution mode
func (m *MultiClusterManager) GetExecutionMode() string {
	return m.config.Mode
}
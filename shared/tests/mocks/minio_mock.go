package mocks

import (
	"fmt"
	"sync"
	"time"
)

// MinIOMock provides a mock MinIO service for testing backup storage
type MinIOMock struct {
	mu          sync.RWMutex
	backups     map[string]*MockBackup
	buckets     map[string]*MockBucket
	failureRate float64
	latency     time.Duration
	running     bool
}

// MockBackup represents a backup stored in mock MinIO
type MockBackup struct {
	ID            string                 `json:"id"`
	ClusterName   string                 `json:"cluster_name"`
	Timestamp     time.Time              `json:"timestamp"`
	Size          int64                  `json:"size"`
	ResourceCount int                    `json:"resource_count"`
	Namespaces    []string               `json:"namespaces"`
	Status        string                 `json:"status"`
	Path          string                 `json:"path"`
	Metadata      map[string]interface{} `json:"metadata"`
	Data          []byte                 `json:"data"`
	Checksum      string                 `json:"checksum"`
}

// MockBucket represents a bucket in mock MinIO
type MockBucket struct {
	Name         string                 `json:"name"`
	CreatedAt    time.Time              `json:"created_at"`
	Size         int64                  `json:"size"`
	ObjectCount  int                    `json:"object_count"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// NewMinIOMock creates a new MinIO mock service
func NewMinIOMock() *MinIOMock {
	return &MinIOMock{
		backups:     make(map[string]*MockBackup),
		buckets:     make(map[string]*MockBucket),
		failureRate: 0.0,
		latency:     100 * time.Millisecond,
		running:     false,
	}
}

// Start starts the mock MinIO service
func (m *MinIOMock) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.running {
		return fmt.Errorf("MinIO mock is already running")
	}
	
	// Create default bucket
	m.buckets["test-backups"] = &MockBucket{
		Name:        "test-backups",
		CreatedAt:   time.Now(),
		Size:        0,
		ObjectCount: 0,
		Metadata:    make(map[string]interface{}),
	}
	
	m.running = true
	return nil
}

// Stop stops the mock MinIO service
func (m *MinIOMock) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.running = false
	return nil
}

// Reset resets the mock MinIO service state
func (m *MinIOMock) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.backups = make(map[string]*MockBackup)
	m.buckets = make(map[string]*MockBucket)
	
	// Recreate default bucket
	if m.running {
		m.buckets["test-backups"] = &MockBucket{
			Name:        "test-backups",
			CreatedAt:   time.Now(),
			Size:        0,
			ObjectCount: 0,
			Metadata:    make(map[string]interface{}),
		}
	}
}

// SetFailureRate sets the failure rate for operations (0.0 to 1.0)
func (m *MinIOMock) SetFailureRate(rate float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failureRate = rate
}

// SetLatency sets the simulated latency for operations
func (m *MinIOMock) SetLatency(latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latency = latency
}

// AddBackup adds a backup to the mock storage
func (m *MinIOMock) AddBackup(backup BackupMetadata) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if !m.running {
		return fmt.Errorf("MinIO mock is not running")
	}
	
	// Simulate latency
	time.Sleep(m.latency)
	
	// Simulate failure
	if m.shouldFail() {
		return fmt.Errorf("simulated MinIO failure")
	}
	
	// Generate mock backup data
	mockData := m.generateMockBackupData(backup)
	
	mockBackup := &MockBackup{
		ID:            backup.ID,
		ClusterName:   backup.ClusterName,
		Timestamp:     backup.Timestamp,
		Size:          backup.Size,
		ResourceCount: backup.ResourceCount,
		Namespaces:    backup.Namespaces,
		Status:        backup.Status,
		Path:          backup.Path,
		Data:          mockData,
		Checksum:      m.calculateChecksum(mockData),
		Metadata: map[string]interface{}{
			"created_by": "test-system",
			"version":    "1.0.0",
			"compression": "gzip",
		},
	}
	
	m.backups[backup.ID] = mockBackup
	
	// Update bucket stats
	if bucket, exists := m.buckets["test-backups"]; exists {
		bucket.Size += backup.Size
		bucket.ObjectCount++
	}
	
	return nil
}

// GetBackup retrieves a backup from mock storage
func (m *MinIOMock) GetBackup(backupID string) (*MockBackup, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if !m.running {
		return nil, fmt.Errorf("MinIO mock is not running")
	}
	
	// Simulate latency
	time.Sleep(m.latency)
	
	// Simulate failure
	if m.shouldFail() {
		return nil, fmt.Errorf("simulated MinIO failure")
	}
	
	backup, exists := m.backups[backupID]
	if !exists {
		return nil, fmt.Errorf("backup %s not found", backupID)
	}
	
	return backup, nil
}

// ListBackups lists all backups in mock storage
func (m *MinIOMock) ListBackups() ([]*MockBackup, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if !m.running {
		return nil, fmt.Errorf("MinIO mock is not running")
	}
	
	// Simulate latency
	time.Sleep(m.latency)
	
	// Simulate failure
	if m.shouldFail() {
		return nil, fmt.Errorf("simulated MinIO failure")
	}
	
	backups := make([]*MockBackup, 0, len(m.backups))
	for _, backup := range m.backups {
		backups = append(backups, backup)
	}
	
	return backups, nil
}

// DeleteBackup deletes a backup from mock storage
func (m *MinIOMock) DeleteBackup(backupID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if !m.running {
		return fmt.Errorf("MinIO mock is not running")
	}
	
	// Simulate latency
	time.Sleep(m.latency)
	
	// Simulate failure
	if m.shouldFail() {
		return fmt.Errorf("simulated MinIO failure")
	}
	
	backup, exists := m.backups[backupID]
	if !exists {
		return fmt.Errorf("backup %s not found", backupID)
	}
	
	// Update bucket stats
	if bucket, exists := m.buckets["test-backups"]; exists {
		bucket.Size -= backup.Size
		bucket.ObjectCount--
	}
	
	delete(m.backups, backupID)
	return nil
}

// ValidateBackup validates backup integrity
func (m *MinIOMock) ValidateBackup(backupID string) (*BackupValidationResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if !m.running {
		return nil, fmt.Errorf("MinIO mock is not running")
	}
	
	// Simulate latency
	time.Sleep(m.latency * 2) // Validation takes longer
	
	// Simulate failure
	if m.shouldFail() {
		return nil, fmt.Errorf("simulated MinIO failure")
	}
	
	backup, exists := m.backups[backupID]
	if !exists {
		return nil, fmt.Errorf("backup %s not found", backupID)
	}
	
	// Simulate validation checks
	result := &BackupValidationResult{
		BackupID:    backupID,
		Valid:       true,
		Timestamp:   time.Now(),
		Checks: map[string]string{
			"integrity":     "passed",
			"completeness":  "passed",
			"compatibility": "passed",
			"checksum":      "passed",
		},
		Warnings: []string{},
		Errors:   []string{},
	}
	
	// Add warnings based on backup age
	age := time.Since(backup.Timestamp)
	if age > 30*24*time.Hour {
		result.Warnings = append(result.Warnings, "Backup is older than 30 days")
	}
	
	if age > 90*24*time.Hour {
		result.Warnings = append(result.Warnings, "Backup is older than 90 days - consider verification")
	}
	
	// Simulate occasional validation failures
	if backup.Status != "completed" {
		result.Valid = false
		result.Errors = append(result.Errors, "Backup is not in completed state")
		result.Checks["completeness"] = "failed"
	}
	
	return result, nil
}

// GetBackupData retrieves backup data (for restore operations)
func (m *MinIOMock) GetBackupData(backupID string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if !m.running {
		return nil, fmt.Errorf("MinIO mock is not running")
	}
	
	// Simulate latency for data retrieval
	time.Sleep(m.latency * 3)
	
	// Simulate failure
	if m.shouldFail() {
		return nil, fmt.Errorf("simulated MinIO failure")
	}
	
	backup, exists := m.backups[backupID]
	if !exists {
		return nil, fmt.Errorf("backup %s not found", backupID)
	}
	
	return backup.Data, nil
}

// CreateBucket creates a new bucket
func (m *MinIOMock) CreateBucket(bucketName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if !m.running {
		return fmt.Errorf("MinIO mock is not running")
	}
	
	// Simulate latency
	time.Sleep(m.latency)
	
	// Simulate failure
	if m.shouldFail() {
		return fmt.Errorf("simulated MinIO failure")
	}
	
	if _, exists := m.buckets[bucketName]; exists {
		return fmt.Errorf("bucket %s already exists", bucketName)
	}
	
	m.buckets[bucketName] = &MockBucket{
		Name:        bucketName,
		CreatedAt:   time.Now(),
		Size:        0,
		ObjectCount: 0,
		Metadata:    make(map[string]interface{}),
	}
	
	return nil
}

// ListBuckets lists all buckets
func (m *MinIOMock) ListBuckets() ([]*MockBucket, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if !m.running {
		return nil, fmt.Errorf("MinIO mock is not running")
	}
	
	// Simulate latency
	time.Sleep(m.latency)
	
	// Simulate failure
	if m.shouldFail() {
		return nil, fmt.Errorf("simulated MinIO failure")
	}
	
	buckets := make([]*MockBucket, 0, len(m.buckets))
	for _, bucket := range m.buckets {
		buckets = append(buckets, bucket)
	}
	
	return buckets, nil
}

// GetStorageStats returns storage statistics
func (m *MinIOMock) GetStorageStats() (*StorageStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if !m.running {
		return nil, fmt.Errorf("MinIO mock is not running")
	}
	
	// Simulate latency
	time.Sleep(m.latency)
	
	var totalSize int64
	var totalObjects int
	
	for _, bucket := range m.buckets {
		totalSize += bucket.Size
		totalObjects += bucket.ObjectCount
	}
	
	return &StorageStats{
		TotalSize:     totalSize,
		TotalObjects:  totalObjects,
		TotalBuckets:  len(m.buckets),
		UsedSpace:     totalSize,
		FreeSpace:     1024 * 1024 * 1024 * 1024, // 1TB free space
		LastUpdated:   time.Now(),
	}, nil
}

// SimulateFailure forces the next operation to fail
func (m *MinIOMock) SimulateFailure() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failureRate = 1.0
}

// SimulateSlowness increases latency for operations
func (m *MinIOMock) SimulateSlowness(factor int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latency = m.latency * time.Duration(factor)
}

// IsRunning returns whether the mock service is running
func (m *MinIOMock) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// GetBackupCount returns the number of backups stored
func (m *MinIOMock) GetBackupCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.backups)
}

// Helper methods

func (m *MinIOMock) shouldFail() bool {
	if m.failureRate <= 0 {
		return false
	}
	if m.failureRate >= 1.0 {
		m.failureRate = 0.0 // Reset after one failure
		return true
	}
	// Simple random failure simulation
	return time.Now().UnixNano()%100 < int64(m.failureRate*100)
}

func (m *MinIOMock) generateMockBackupData(backup BackupMetadata) []byte {
	// Generate mock Kubernetes YAML data
	data := fmt.Sprintf(`# Mock backup data for %s
# Generated at: %s
# Cluster: %s
# Namespaces: %v

apiVersion: v1
kind: Namespace
metadata:
  name: %s
  labels:
    backup-id: %s
    cluster: %s

---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mock-app
  namespace: %s
  labels:
    backup-id: %s
spec:
  replicas: 3
  selector:
    matchLabels:
      app: mock-app
  template:
    metadata:
      labels:
        app: mock-app
    spec:
      containers:
      - name: app
        image: nginx:1.21
        ports:
        - containerPort: 80

---
apiVersion: v1
kind: Service
metadata:
  name: mock-app-service
  namespace: %s
  labels:
    backup-id: %s
spec:
  selector:
    app: mock-app
  ports:
  - port: 80
    targetPort: 80
  type: ClusterIP
`,
		backup.ID,
		backup.Timestamp.Format(time.RFC3339),
		backup.ClusterName,
		backup.Namespaces,
		backup.Namespaces[0],
		backup.ID,
		backup.ClusterName,
		backup.Namespaces[0],
		backup.ID,
		backup.Namespaces[0],
		backup.ID,
	)
	
	return []byte(data)
}

func (m *MinIOMock) calculateChecksum(data []byte) string {
	// Simple checksum calculation (in real implementation would use proper hashing)
	sum := int64(0)
	for _, b := range data {
		sum += int64(b)
	}
	return fmt.Sprintf("mock-checksum-%d", sum)
}

// Support types

type BackupMetadata struct {
	ID            string    `json:"id"`
	ClusterName   string    `json:"cluster_name"`
	Timestamp     time.Time `json:"timestamp"`
	Size          int64     `json:"size"`
	ResourceCount int       `json:"resource_count"`
	Namespaces    []string  `json:"namespaces"`
	Status        string    `json:"status"`
	Path          string    `json:"path"`
}

type BackupValidationResult struct {
	BackupID  string            `json:"backup_id"`
	Valid     bool              `json:"valid"`
	Timestamp time.Time         `json:"timestamp"`
	Checks    map[string]string `json:"checks"`
	Warnings  []string          `json:"warnings"`
	Errors    []string          `json:"errors"`
}

type StorageStats struct {
	TotalSize    int64     `json:"total_size"`
	TotalObjects int       `json:"total_objects"`
	TotalBuckets int       `json:"total_buckets"`
	UsedSpace    int64     `json:"used_space"`
	FreeSpace    int64     `json:"free_space"`
	LastUpdated  time.Time `json:"last_updated"`
}
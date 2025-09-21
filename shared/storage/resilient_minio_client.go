package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	sharedconfig "shared-config/config"
	"shared-config/monitoring"
	"shared-config/resilience"
)

// ResilientMinIOClient wraps MinIO client with circuit breaker protection
type ResilientMinIOClient struct {
	client                *minio.Client
	circuitBreakerManager *resilience.CircuitBreakerManager
	config                *sharedconfig.StorageConfig
	monitoring            monitoring.MetricsCollector
	serviceName           string
}

// MinIOOperationMetrics tracks MinIO operation performance
type MinIOOperationMetrics struct {
	TotalOperations      int64
	SuccessfulOps        int64
	FailedOps            int64
	CircuitBreakerRejects int64
	BucketOperations     int64
	ObjectOperations     int64
	UploadOperations     int64
	DownloadOperations   int64
	AverageLatency       time.Duration
	TotalBytesUploaded   int64
	TotalBytesDownloaded int64
}

// NewResilientMinIOClient creates a new resilient MinIO client
func NewResilientMinIOClient(
	config *sharedconfig.StorageConfig,
	circuitBreakerManager *resilience.CircuitBreakerManager,
	monitoring monitoring.MetricsCollector,
) (*ResilientMinIOClient, error) {
	if config == nil {
		return nil, fmt.Errorf("storage config is required")
	}
	
	// Create MinIO client
	minioClient, err := minio.New(config.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(config.AccessKey, config.SecretKey, ""),
		Secure: config.UseSSL,
		Region: config.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %v", err)
	}
	
	serviceName := "minio"
	
	return &ResilientMinIOClient{
		client:                minioClient,
		circuitBreakerManager: circuitBreakerManager,
		config:                config,
		monitoring:            monitoring,
		serviceName:           serviceName,
	}, nil
}

// BucketExists checks if a bucket exists with circuit breaker protection
func (rc *ResilientMinIOClient) BucketExists(ctx context.Context, bucketName string) (bool, error) {
	var exists bool
	var err error
	
	cbError := rc.circuitBreakerManager.WrapMinIOOperation(ctx, func() error {
		exists, err = rc.client.BucketExists(ctx, bucketName)
		return err
	})
	
	if cbError != nil {
		rc.recordMetric("minio_bucket_exists_errors", 1)
		return false, cbError
	}
	
	rc.recordMetric("minio_bucket_operations", 1)
	return exists, nil
}

// MakeBucket creates a bucket with circuit breaker protection
func (rc *ResilientMinIOClient) MakeBucket(ctx context.Context, bucketName string, opts minio.MakeBucketOptions) error {
	cbError := rc.circuitBreakerManager.WrapMinIOOperation(ctx, func() error {
		return rc.client.MakeBucket(ctx, bucketName, opts)
	})
	
	if cbError != nil {
		rc.recordMetric("minio_make_bucket_errors", 1)
		return cbError
	}
	
	rc.recordMetric("minio_bucket_operations", 1)
	return nil
}

// RemoveBucket removes a bucket with circuit breaker protection
func (rc *ResilientMinIOClient) RemoveBucket(ctx context.Context, bucketName string) error {
	cbError := rc.circuitBreakerManager.WrapMinIOOperation(ctx, func() error {
		return rc.client.RemoveBucket(ctx, bucketName)
	})
	
	if cbError != nil {
		rc.recordMetric("minio_remove_bucket_errors", 1)
		return cbError
	}
	
	rc.recordMetric("minio_bucket_operations", 1)
	return nil
}

// ListBuckets lists all buckets with circuit breaker protection
func (rc *ResilientMinIOClient) ListBuckets(ctx context.Context) ([]minio.BucketInfo, error) {
	var buckets []minio.BucketInfo
	var err error
	
	cbError := rc.circuitBreakerManager.WrapMinIOOperation(ctx, func() error {
		buckets, err = rc.client.ListBuckets(ctx)
		return err
	})
	
	if cbError != nil {
		rc.recordMetric("minio_list_buckets_errors", 1)
		return nil, cbError
	}
	
	rc.recordMetric("minio_bucket_operations", 1)
	return buckets, nil
}

// PutObject uploads an object with circuit breaker protection
func (rc *ResilientMinIOClient) PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	var uploadInfo minio.UploadInfo
	var err error
	
	start := time.Now()
	cbError := rc.circuitBreakerManager.WrapMinIOOperation(ctx, func() error {
		uploadInfo, err = rc.client.PutObject(ctx, bucketName, objectName, reader, objectSize, opts)
		return err
	})
	duration := time.Since(start)
	
	if cbError != nil {
		rc.recordMetric("minio_put_object_errors", 1)
		rc.recordDuration("minio_put_object_duration", duration, "error")
		return minio.UploadInfo{}, cbError
	}
	
	rc.recordMetric("minio_object_operations", 1)
	rc.recordMetric("minio_upload_operations", 1)
	rc.recordMetric("minio_bytes_uploaded", float64(objectSize))
	rc.recordDuration("minio_put_object_duration", duration, "success")
	
	return uploadInfo, nil
}

// GetObject downloads an object with circuit breaker protection
func (rc *ResilientMinIOClient) GetObject(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (*minio.Object, error) {
	var object *minio.Object
	var err error
	
	start := time.Now()
	cbError := rc.circuitBreakerManager.WrapMinIOOperation(ctx, func() error {
		object, err = rc.client.GetObject(ctx, bucketName, objectName, opts)
		return err
	})
	duration := time.Since(start)
	
	if cbError != nil {
		rc.recordMetric("minio_get_object_errors", 1)
		rc.recordDuration("minio_get_object_duration", duration, "error")
		return nil, cbError
	}
	
	rc.recordMetric("minio_object_operations", 1)
	rc.recordMetric("minio_download_operations", 1)
	rc.recordDuration("minio_get_object_duration", duration, "success")
	
	return object, nil
}

// FGetObject downloads an object to file with circuit breaker protection
func (rc *ResilientMinIOClient) FGetObject(ctx context.Context, bucketName, objectName, filePath string, opts minio.GetObjectOptions) error {
	start := time.Now()
	cbError := rc.circuitBreakerManager.WrapMinIOOperation(ctx, func() error {
		return rc.client.FGetObject(ctx, bucketName, objectName, filePath, opts)
	})
	duration := time.Since(start)
	
	if cbError != nil {
		rc.recordMetric("minio_fget_object_errors", 1)
		rc.recordDuration("minio_fget_object_duration", duration, "error")
		return cbError
	}
	
	rc.recordMetric("minio_object_operations", 1)
	rc.recordMetric("minio_download_operations", 1)
	rc.recordDuration("minio_fget_object_duration", duration, "success")
	
	return nil
}

// FPutObject uploads a file with circuit breaker protection
func (rc *ResilientMinIOClient) FPutObject(ctx context.Context, bucketName, objectName, filePath string, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	var uploadInfo minio.UploadInfo
	var err error
	
	start := time.Now()
	cbError := rc.circuitBreakerManager.WrapMinIOOperation(ctx, func() error {
		uploadInfo, err = rc.client.FPutObject(ctx, bucketName, objectName, filePath, opts)
		return err
	})
	duration := time.Since(start)
	
	if cbError != nil {
		rc.recordMetric("minio_fput_object_errors", 1)
		rc.recordDuration("minio_fput_object_duration", duration, "error")
		return minio.UploadInfo{}, cbError
	}
	
	rc.recordMetric("minio_object_operations", 1)
	rc.recordMetric("minio_upload_operations", 1)
	rc.recordMetric("minio_bytes_uploaded", float64(uploadInfo.Size))
	rc.recordDuration("minio_fput_object_duration", duration, "success")
	
	return uploadInfo, nil
}

// StatObject gets object metadata with circuit breaker protection
func (rc *ResilientMinIOClient) StatObject(ctx context.Context, bucketName, objectName string, opts minio.StatObjectOptions) (minio.ObjectInfo, error) {
	var objectInfo minio.ObjectInfo
	var err error
	
	cbError := rc.circuitBreakerManager.WrapMinIOOperation(ctx, func() error {
		objectInfo, err = rc.client.StatObject(ctx, bucketName, objectName, opts)
		return err
	})
	
	if cbError != nil {
		rc.recordMetric("minio_stat_object_errors", 1)
		return minio.ObjectInfo{}, cbError
	}
	
	rc.recordMetric("minio_object_operations", 1)
	return objectInfo, nil
}

// RemoveObject removes an object with circuit breaker protection
func (rc *ResilientMinIOClient) RemoveObject(ctx context.Context, bucketName, objectName string, opts minio.RemoveObjectOptions) error {
	cbError := rc.circuitBreakerManager.WrapMinIOOperation(ctx, func() error {
		return rc.client.RemoveObject(ctx, bucketName, objectName, opts)
	})
	
	if cbError != nil {
		rc.recordMetric("minio_remove_object_errors", 1)
		return cbError
	}
	
	rc.recordMetric("minio_object_operations", 1)
	return nil
}

// RemoveObjects removes multiple objects with circuit breaker protection
func (rc *ResilientMinIOClient) RemoveObjects(ctx context.Context, bucketName string, objectsCh <-chan minio.ObjectInfo, opts minio.RemoveObjectsOptions) <-chan minio.RemoveObjectError {
	// This operation is more complex as it's streaming
	// We'll wrap the entire operation in circuit breaker
	errorCh := make(chan minio.RemoveObjectError)
	
	go func() {
		defer close(errorCh)
		
		cbError := rc.circuitBreakerManager.WrapMinIOOperation(ctx, func() error {
			removeErrorCh := rc.client.RemoveObjects(ctx, bucketName, objectsCh, opts)
			
			// Forward errors from MinIO client
			for removeErr := range removeErrorCh {
				errorCh <- removeErr
			}
			
			return nil
		})
		
		if cbError != nil {
			// Create error for circuit breaker rejection
			errorCh <- minio.RemoveObjectError{
				Err: cbError,
			}
			rc.recordMetric("minio_remove_objects_errors", 1)
			return
		}
		
		rc.recordMetric("minio_object_operations", 1)
	}()
	
	return errorCh
}

// ListObjects lists objects with circuit breaker protection
func (rc *ResilientMinIOClient) ListObjects(ctx context.Context, bucketName string, opts minio.ListObjectsOptions) <-chan minio.ObjectInfo {
	objectCh := make(chan minio.ObjectInfo)
	
	go func() {
		defer close(objectCh)
		
		cbError := rc.circuitBreakerManager.WrapMinIOOperation(ctx, func() error {
			// Get the object channel from MinIO client
			minioObjectCh := rc.client.ListObjects(ctx, bucketName, opts)
			
			// Forward objects from MinIO client
			for object := range minioObjectCh {
				if object.Err != nil {
					// MinIO client encountered an error
					return object.Err
				}
				
				select {
				case <-ctx.Done():
					return ctx.Err()
				case objectCh <- object:
				}
			}
			
			return nil
		})
		
		if cbError != nil {
			// Send error object for circuit breaker rejection
			select {
			case <-ctx.Done():
			case objectCh <- minio.ObjectInfo{Err: cbError}:
			}
			rc.recordMetric("minio_list_objects_errors", 1)
			return
		}
		
		rc.recordMetric("minio_object_operations", 1)
	}()
	
	return objectCh
}

// CopyObject copies an object with circuit breaker protection
func (rc *ResilientMinIOClient) CopyObject(ctx context.Context, dst minio.CopyDestOptions, src minio.CopySrcOptions) (minio.UploadInfo, error) {
	var uploadInfo minio.UploadInfo
	var err error
	
	start := time.Now()
	cbError := rc.circuitBreakerManager.WrapMinIOOperation(ctx, func() error {
		uploadInfo, err = rc.client.CopyObject(ctx, dst, src)
		return err
	})
	duration := time.Since(start)
	
	if cbError != nil {
		rc.recordMetric("minio_copy_object_errors", 1)
		rc.recordDuration("minio_copy_object_duration", duration, "error")
		return minio.UploadInfo{}, cbError
	}
	
	rc.recordMetric("minio_object_operations", 1)
	rc.recordDuration("minio_copy_object_duration", duration, "success")
	
	return uploadInfo, nil
}

// HealthCheck performs a health check on the MinIO service
func (rc *ResilientMinIOClient) HealthCheck(ctx context.Context) error {
	// Simple health check by listing buckets with a short timeout
	healthCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	
	return rc.circuitBreakerManager.WrapMinIOOperation(healthCtx, func() error {
		_, err := rc.client.ListBuckets(healthCtx)
		return err
	})
}

// GetCircuitBreakerState returns the current circuit breaker state
func (rc *ResilientMinIOClient) GetCircuitBreakerState() resilience.CircuitBreakerState {
	cb := rc.circuitBreakerManager.GetServiceCircuitBreaker(resilience.ServiceMinIO)
	return cb.GetState()
}

// IsHealthy returns true if the circuit breaker is not in OPEN state
func (rc *ResilientMinIOClient) IsHealthy() bool {
	return rc.GetCircuitBreakerState() != resilience.StateOpen
}

// GetMetrics returns comprehensive metrics for MinIO operations
func (rc *ResilientMinIOClient) GetMetrics() map[string]interface{} {
	cb := rc.circuitBreakerManager.GetServiceCircuitBreaker(resilience.ServiceMinIO)
	cbMetrics := cb.GetMetrics()
	
	return map[string]interface{}{
		"circuit_breaker": map[string]interface{}{
			"state":              cbMetrics.State.String(),
			"total_requests":     cbMetrics.TotalRequests,
			"successful_requests": cbMetrics.SuccessfulReqs,
			"failed_requests":    cbMetrics.FailedReqs,
			"rejected_requests":  cbMetrics.RejectedReqs,
			"failure_streak":     cbMetrics.FailureStreak,
			"last_failure":       cbMetrics.LastFailureTime,
			"last_success":       cbMetrics.LastSuccessTime,
		},
		"service_name": rc.serviceName,
		"endpoint":     rc.config.Endpoint,
		"bucket":       rc.config.Bucket,
		"timestamp":    time.Now(),
	}
}

// GetHealthStatus returns detailed health information
func (rc *ResilientMinIOClient) GetHealthStatus() map[string]interface{} {
	state := rc.GetCircuitBreakerState()
	metrics := rc.GetMetrics()
	
	cbMetrics := metrics["circuit_breaker"].(map[string]interface{})
	
	successRate := float64(0)
	if totalReqs := cbMetrics["total_requests"].(int64); totalReqs > 0 {
		successRate = float64(cbMetrics["successful_requests"].(int64)) / float64(totalReqs) * 100
	}
	
	return map[string]interface{}{
		"service":       "minio",
		"endpoint":      rc.config.Endpoint,
		"bucket":        rc.config.Bucket,
		"healthy":       state != resilience.StateOpen,
		"state":         state.String(),
		"success_rate":  successRate,
		"total_requests": cbMetrics["total_requests"],
		"recent_failures": cbMetrics["failure_streak"],
		"last_failure":  cbMetrics["last_failure"],
		"timestamp":     time.Now(),
	}
}

// ResetCircuitBreaker resets the circuit breaker for MinIO operations
func (rc *ResilientMinIOClient) ResetCircuitBreaker() error {
	return rc.circuitBreakerManager.ResetCircuitBreaker(rc.serviceName)
}

// ForceOpenCircuitBreaker forces the circuit breaker to open state
func (rc *ResilientMinIOClient) ForceOpenCircuitBreaker() error {
	return rc.circuitBreakerManager.ForceOpenCircuitBreaker(rc.serviceName)
}

// Private helper methods

func (rc *ResilientMinIOClient) recordMetric(metricName string, value float64) {
	if rc.monitoring == nil {
		return
	}
	
	labels := map[string]string{
		"service":  rc.serviceName,
		"endpoint": rc.config.Endpoint,
		"bucket":   rc.config.Bucket,
	}
	rc.monitoring.IncCounter(metricName, labels, value)
}

func (rc *ResilientMinIOClient) recordDuration(metricName string, duration time.Duration, status string) {
	if rc.monitoring == nil {
		return
	}
	
	labels := map[string]string{
		"service":  rc.serviceName,
		"endpoint": rc.config.Endpoint,
		"bucket":   rc.config.Bucket,
		"status":   status,
	}
	rc.monitoring.RecordDuration(metricName, labels, duration)
}

// Helper function to create resilient MinIO client from shared config
func NewResilientMinIOClientFromSharedConfig(
	sharedConfig *sharedconfig.SharedConfig,
	circuitBreakerManager *resilience.CircuitBreakerManager,
	monitoring monitoring.MetricsCollector,
) (*ResilientMinIOClient, error) {
	if sharedConfig == nil {
		return nil, fmt.Errorf("shared config is required")
	}
	
	return NewResilientMinIOClient(&sharedConfig.Storage, circuitBreakerManager, monitoring)
}
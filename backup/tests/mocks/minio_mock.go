package mocks

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
)

// MockMinioClient is a mock implementation of MinIO client for testing
type MockMinioClient struct {
	buckets       map[string]bool
	objects       map[string]map[string][]byte
	shouldError   bool
	errorMessage  string
	callLog       []string
}

// NewMockMinioClient creates a new mock MinIO client
func NewMockMinioClient() *MockMinioClient {
	return &MockMinioClient{
		buckets: make(map[string]bool),
		objects: make(map[string]map[string][]byte),
		callLog: make([]string, 0),
	}
}

// SetError configures the mock to return errors
func (m *MockMinioClient) SetError(shouldError bool, message string) {
	m.shouldError = shouldError
	m.errorMessage = message
}

// GetCallLog returns the log of method calls
func (m *MockMinioClient) GetCallLog() []string {
	return m.callLog
}

// BucketExists checks if a bucket exists
func (m *MockMinioClient) BucketExists(ctx context.Context, bucketName string) (bool, error) {
	m.callLog = append(m.callLog, fmt.Sprintf("BucketExists(%s)", bucketName))
	
	if m.shouldError {
		return false, fmt.Errorf(m.errorMessage)
	}
	
	exists, ok := m.buckets[bucketName]
	return ok && exists, nil
}

// MakeBucket creates a new bucket
func (m *MockMinioClient) MakeBucket(ctx context.Context, bucketName string, opts minio.MakeBucketOptions) error {
	m.callLog = append(m.callLog, fmt.Sprintf("MakeBucket(%s)", bucketName))
	
	if m.shouldError {
		return fmt.Errorf(m.errorMessage)
	}
	
	m.buckets[bucketName] = true
	m.objects[bucketName] = make(map[string][]byte)
	return nil
}

// PutObject uploads an object to the bucket
func (m *MockMinioClient) PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	m.callLog = append(m.callLog, fmt.Sprintf("PutObject(%s, %s)", bucketName, objectName))
	
	if m.shouldError {
		return minio.UploadInfo{}, fmt.Errorf(m.errorMessage)
	}
	
	// Read all data from reader
	data, err := io.ReadAll(reader)
	if err != nil {
		return minio.UploadInfo{}, err
	}
	
	// Ensure bucket exists
	if !m.buckets[bucketName] {
		return minio.UploadInfo{}, fmt.Errorf("bucket %s does not exist", bucketName)
	}
	
	// Store object
	m.objects[bucketName][objectName] = data
	
	return minio.UploadInfo{
		Size:         int64(len(data)),
		ETag:         "mock-etag",
		LastModified: time.Now(),
	}, nil
}

// GetObject retrieves an object from the bucket
func (m *MockMinioClient) GetObject(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (*minio.Object, error) {
	m.callLog = append(m.callLog, fmt.Sprintf("GetObject(%s, %s)", bucketName, objectName))
	
	if m.shouldError {
		return nil, fmt.Errorf(m.errorMessage)
	}
	
	// This is a simplified mock - in real tests you might want to return a proper Object
	return nil, nil
}

// ListObjects lists objects in a bucket
func (m *MockMinioClient) ListObjects(ctx context.Context, bucketName string, opts minio.ListObjectsOptions) <-chan minio.ObjectInfo {
	m.callLog = append(m.callLog, fmt.Sprintf("ListObjects(%s)", bucketName))
	
	objectsChan := make(chan minio.ObjectInfo, 1)
	
	go func() {
		defer close(objectsChan)
		
		if m.shouldError {
			objectsChan <- minio.ObjectInfo{
				Err: fmt.Errorf(m.errorMessage),
			}
			return
		}
		
		bucket, exists := m.objects[bucketName]
		if !exists {
			return
		}
		
		for objectName, data := range bucket {
			if opts.Prefix == "" || len(objectName) >= len(opts.Prefix) && objectName[:len(opts.Prefix)] == opts.Prefix {
				objectsChan <- minio.ObjectInfo{
					Key:          objectName,
					Size:         int64(len(data)),
					LastModified: time.Now(),
				}
			}
		}
	}()
	
	return objectsChan
}

// RemoveObject removes an object from the bucket
func (m *MockMinioClient) RemoveObject(ctx context.Context, bucketName, objectName string, opts minio.RemoveObjectOptions) error {
	m.callLog = append(m.callLog, fmt.Sprintf("RemoveObject(%s, %s)", bucketName, objectName))
	
	if m.shouldError {
		return fmt.Errorf(m.errorMessage)
	}
	
	bucket, exists := m.objects[bucketName]
	if !exists {
		return fmt.Errorf("bucket %s does not exist", bucketName)
	}
	
	delete(bucket, objectName)
	return nil
}

// AddTestBucket adds a bucket for testing
func (m *MockMinioClient) AddTestBucket(bucketName string) {
	m.buckets[bucketName] = true
	m.objects[bucketName] = make(map[string][]byte)
}

// AddTestObject adds an object for testing
func (m *MockMinioClient) AddTestObject(bucketName, objectName string, data []byte) {
	if !m.buckets[bucketName] {
		m.AddTestBucket(bucketName)
	}
	m.objects[bucketName][objectName] = data
}

// GetTestObject retrieves a test object's data
func (m *MockMinioClient) GetTestObject(bucketName, objectName string) ([]byte, bool) {
	bucket, bucketExists := m.objects[bucketName]
	if !bucketExists {
		return nil, false
	}
	
	data, objectExists := bucket[objectName]
	return data, objectExists
}

// GetObjectCount returns the number of objects in a bucket
func (m *MockMinioClient) GetObjectCount(bucketName string) int {
	bucket, exists := m.objects[bucketName]
	if !exists {
		return 0
	}
	return len(bucket)
}
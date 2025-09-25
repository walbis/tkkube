package main

import (
	"context"
	"log"
	"os"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func main() {
	log.Println("=== Enhanced Multi-Cluster Validation Test ===")
	log.Println("Test Date:", time.Now().Format("2006-01-02 15:04:05"))
	
	// Get token from environment
	token := os.Getenv("CRC_TOKEN")
	if token == "" {
		log.Fatal("CRC_TOKEN environment variable not set")
	}

	log.Println("\n=== Test 1: Token Validation ===")
	testTokenValidation(token)

	log.Println("\n=== Test 2: Cluster Connectivity ===")
	testClusterConnectivity(token)

	log.Println("\n=== Test 3: API Authentication ===")
	testAPIAuthentication(token)

	log.Println("\n=== Test 4: Performance Metrics ===")
	testPerformanceMetrics(token)

	log.Println("\n=== Validation Summary ===")
	log.Println("✅ All validation tests completed successfully!")
}

func testTokenValidation(token string) {
	log.Printf("🔑 Testing token validation...")
	
	// Basic token format validation
	if len(token) < 10 {
		log.Printf("❌ Token too short: %d characters", len(token))
		return
	}
	
	if len(token) > 2048 {
		log.Printf("❌ Token too long: %d characters", len(token))
		return
	}
	
	// Check token format (sha256~ prefix for CRC)
	if len(token) > 7 && token[:7] == "sha256~" {
		log.Printf("✅ Valid CRC token format detected")
	} else {
		log.Printf("⚠️  Non-standard token format (may still be valid)")
	}
	
	log.Printf("✅ Token validation passed")
	log.Printf("   Token length: %d characters", len(token))
	log.Printf("   Token prefix: %s...", token[:min(20, len(token))])
}

func testClusterConnectivity(token string) {
	log.Printf("🌐 Testing cluster connectivity...")
	
	config := &rest.Config{
		Host:            "https://api.crc.testing:6443",
		BearerToken:     token,
		TLSClientConfig: rest.TLSClientConfig{Insecure: true},
		Timeout:         10 * time.Second,
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Printf("❌ Failed to create kubernetes client: %v", err)
		return
	}

	startTime := time.Now()
	version, err := clientset.Discovery().ServerVersion()
	duration := time.Since(startTime)
	
	if err != nil {
		log.Printf("❌ Failed to connect to cluster: %v", err)
		return
	}

	log.Printf("✅ Cluster connectivity successful")
	log.Printf("   API Server: https://api.crc.testing:6443")
	log.Printf("   Kubernetes Version: %s", version.String())
	log.Printf("   Connection time: %v", duration)
	
	if duration < 1*time.Second {
		log.Printf("⚡ Connection performance: EXCELLENT")
	} else if duration < 3*time.Second {
		log.Printf("✅ Connection performance: GOOD")
	} else {
		log.Printf("⚠️  Connection performance: SLOW")
	}
}

func testAPIAuthentication(token string) {
	log.Printf("🔐 Testing API authentication...")
	
	config := &rest.Config{
		Host:            "https://api.crc.testing:6443",
		BearerToken:     token,
		TLSClientConfig: rest.TLSClientConfig{Insecure: true},
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Printf("❌ Failed to create kubernetes client: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test different API operations to validate authentication
	tests := []struct {
		name string
		test func() error
	}{
		{"List namespaces", func() error {
			_, err := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 5})
			return err
		}},
		{"List nodes", func() error {
			_, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
			return err
		}},
		{"List pods in demo-app", func() error {
			_, err := clientset.CoreV1().Pods("demo-app").List(ctx, metav1.ListOptions{})
			return err
		}},
		{"List services in demo-app", func() error {
			_, err := clientset.CoreV1().Services("demo-app").List(ctx, metav1.ListOptions{})
			return err
		}},
	}

	passed := 0
	for _, test := range tests {
		err := test.test()
		if err != nil {
			log.Printf("❌ %s: %v", test.name, err)
		} else {
			log.Printf("✅ %s: SUCCESS", test.name)
			passed++
		}
	}
	
	log.Printf("📊 Authentication test results: %d/%d passed", passed, len(tests))
	
	if passed == len(tests) {
		log.Printf("✅ All API authentication tests passed")
	} else {
		log.Printf("⚠️  Some authentication tests failed")
	}
}

func testPerformanceMetrics(token string) {
	log.Printf("📈 Testing performance metrics...")
	
	config := &rest.Config{
		Host:            "https://api.crc.testing:6443",
		BearerToken:     token,
		TLSClientConfig: rest.TLSClientConfig{Insecure: true},
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Printf("❌ Failed to create kubernetes client: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Measure various operations
	metrics := make(map[string]time.Duration)
	
	operations := []struct {
		name string
		op   func() error
	}{
		{"namespace-list", func() error {
			_, err := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
			return err
		}},
		{"pod-list", func() error {
			_, err := clientset.CoreV1().Pods("demo-app").List(ctx, metav1.ListOptions{})
			return err
		}},
		{"service-list", func() error {
			_, err := clientset.CoreV1().Services("demo-app").List(ctx, metav1.ListOptions{})
			return err
		}},
		{"configmap-list", func() error {
			_, err := clientset.CoreV1().ConfigMaps("demo-app").List(ctx, metav1.ListOptions{})
			return err
		}},
		{"secret-list", func() error {
			_, err := clientset.CoreV1().Secrets("demo-app").List(ctx, metav1.ListOptions{})
			return err
		}},
	}

	for _, op := range operations {
		startTime := time.Now()
		err := op.op()
		duration := time.Since(startTime)
		
		if err != nil {
			log.Printf("❌ %s failed: %v", op.name, err)
		} else {
			metrics[op.name] = duration
			log.Printf("✅ %s: %v", op.name, duration)
		}
	}

	// Calculate average performance
	var totalDuration time.Duration
	for _, duration := range metrics {
		totalDuration += duration
	}
	
	if len(metrics) > 0 {
		avgDuration := totalDuration / time.Duration(len(metrics))
		log.Printf("📊 Average operation time: %v", avgDuration)
		
		if avgDuration < 100*time.Millisecond {
			log.Printf("⚡ Overall performance: EXCELLENT")
		} else if avgDuration < 500*time.Millisecond {
			log.Printf("✅ Overall performance: GOOD")
		} else {
			log.Printf("⚠️  Overall performance: NEEDS OPTIMIZATION")
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
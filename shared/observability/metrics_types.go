package observability

import (
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Histogram provides a histogram implementation for recording distributions
type Histogram struct {
	buckets     []float64
	counts      []atomic.Int64
	sum         atomic.Value // float64
	count       atomic.Int64
	percentiles []float64
	mu          sync.RWMutex
}

// HistogramSnapshot represents a point-in-time snapshot of histogram data
type HistogramSnapshot struct {
	Count       int64              `json:"count"`
	Sum         float64            `json:"sum"`
	Mean        float64            `json:"mean"`
	Min         float64            `json:"min"`
	Max         float64            `json:"max"`
	Percentiles map[float64]float64 `json:"percentiles"`
	Buckets     map[float64]int64  `json:"buckets"`
}

// NewHistogram creates a new histogram with default buckets
func NewHistogram(percentiles []float64) *Histogram {
	// Default exponential buckets
	buckets := []float64{
		0.001, 0.002, 0.005, 0.01, 0.02, 0.05, 0.1, 0.2, 0.5,
		1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000,
		10000, 20000, 50000, 100000, math.Inf(1),
	}
	
	h := &Histogram{
		buckets:     buckets,
		counts:      make([]atomic.Int64, len(buckets)),
		percentiles: percentiles,
	}
	
	h.sum.Store(0.0)
	
	if len(percentiles) == 0 {
		h.percentiles = []float64{0.5, 0.75, 0.9, 0.95, 0.99, 0.999}
	}
	
	return h
}

// Observe records a value in the histogram
func (h *Histogram) Observe(value float64) {
	// Update sum and count
	h.count.Add(1)
	
	// Atomic add for float64
	for {
		oldSum := h.sum.Load().(float64)
		newSum := oldSum + value
		if h.sum.CompareAndSwap(oldSum, newSum) {
			break
		}
	}
	
	// Find the right bucket
	for i, boundary := range h.buckets {
		if value <= boundary {
			h.counts[i].Add(1)
			break
		}
	}
}

// Snapshot returns a snapshot of the histogram's current state
func (h *Histogram) Snapshot() *HistogramSnapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()
	
	count := h.count.Load()
	sum := h.sum.Load().(float64)
	
	snapshot := &HistogramSnapshot{
		Count:       count,
		Sum:         sum,
		Percentiles: make(map[float64]float64),
		Buckets:     make(map[float64]int64),
	}
	
	if count > 0 {
		snapshot.Mean = sum / float64(count)
		
		// Calculate percentiles from bucket counts
		cumulative := int64(0)
		for i, boundary := range h.buckets {
			bucketCount := h.counts[i].Load()
			cumulative += bucketCount
			snapshot.Buckets[boundary] = bucketCount
			
			// Calculate percentiles
			for _, p := range h.percentiles {
				targetCount := int64(float64(count) * p)
				if cumulative >= targetCount && snapshot.Percentiles[p] == 0 {
					snapshot.Percentiles[p] = boundary
				}
			}
		}
	}
	
	return snapshot
}

// Reset clears the histogram
func (h *Histogram) Reset() {
	h.count.Store(0)
	h.sum.Store(0.0)
	for i := range h.counts {
		h.counts[i].Store(0)
	}
}

// Summary provides a summary implementation for recording distributions
type Summary struct {
	values      []float64
	maxSamples  int
	quantiles   []float64
	mu          sync.Mutex
	count       atomic.Int64
	sum         atomic.Value // float64
}

// SummarySnapshot represents a point-in-time snapshot of summary data
type SummarySnapshot struct {
	Count     int64               `json:"count"`
	Sum       float64             `json:"sum"`
	Mean      float64             `json:"mean"`
	Quantiles map[float64]float64 `json:"quantiles"`
}

// NewSummary creates a new summary with specified quantiles
func NewSummary(quantiles []float64) *Summary {
	s := &Summary{
		values:     make([]float64, 0, 10000),
		maxSamples: 10000,
		quantiles:  quantiles,
	}
	
	s.sum.Store(0.0)
	
	if len(quantiles) == 0 {
		s.quantiles = []float64{0.5, 0.9, 0.95, 0.99}
	}
	
	return s
}

// Observe records a value in the summary
func (s *Summary) Observe(value float64) {
	s.count.Add(1)
	
	// Atomic add for float64
	for {
		oldSum := s.sum.Load().(float64)
		newSum := oldSum + value
		if s.sum.CompareAndSwap(oldSum, newSum) {
			break
		}
	}
	
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Reservoir sampling for large datasets
	if len(s.values) < s.maxSamples {
		s.values = append(s.values, value)
	} else {
		// Random replacement
		idx := int(time.Now().UnixNano() % int64(s.maxSamples))
		s.values[idx] = value
	}
}

// Snapshot returns a snapshot of the summary's current state
func (s *Summary) Snapshot() *SummarySnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	count := s.count.Load()
	sum := s.sum.Load().(float64)
	
	snapshot := &SummarySnapshot{
		Count:     count,
		Sum:       sum,
		Quantiles: make(map[float64]float64),
	}
	
	if count > 0 {
		snapshot.Mean = sum / float64(count)
		
		// Calculate quantiles
		if len(s.values) > 0 {
			sorted := make([]float64, len(s.values))
			copy(sorted, s.values)
			sort.Float64s(sorted)
			
			for _, q := range s.quantiles {
				idx := int(float64(len(sorted)-1) * q)
				snapshot.Quantiles[q] = sorted[idx]
			}
		}
	}
	
	return snapshot
}

// Reset clears the summary
func (s *Summary) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.count.Store(0)
	s.sum.Store(0.0)
	s.values = s.values[:0]
}

// MetricsRegistry manages metric registrations and storage
type MetricsRegistry struct {
	metrics     map[string]*RegisteredMetric
	metricOrder []string
	points      []MetricPoint
	maxPoints   int
	mu          sync.RWMutex
}

// RegisteredMetric represents a registered metric with metadata
type RegisteredMetric struct {
	Name        string            `json:"name"`
	Type        MetricType        `json:"type"`
	Dimensions  map[string]string `json:"dimensions"`
	Created     time.Time         `json:"created"`
	LastUpdated time.Time         `json:"last_updated"`
	Count       int64             `json:"count"`
}

// NewMetricsRegistry creates a new metrics registry
func NewMetricsRegistry() *MetricsRegistry {
	return &MetricsRegistry{
		metrics:   make(map[string]*RegisteredMetric),
		points:    make([]MetricPoint, 0, 10000),
		maxPoints: 100000,
	}
}

// RegisterMetric registers a new metric
func (r *MetricsRegistry) RegisterMetric(name string, metricType MetricType, dimensions map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	key := r.generateKey(name, dimensions)
	if _, exists := r.metrics[key]; !exists {
		r.metrics[key] = &RegisteredMetric{
			Name:        name,
			Type:        metricType,
			Dimensions:  dimensions,
			Created:     time.Now(),
			LastUpdated: time.Now(),
		}
		r.metricOrder = append(r.metricOrder, key)
	}
}

// AddMetricPoint adds a metric point to the registry
func (r *MetricsRegistry) AddMetricPoint(point MetricPoint) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	// Update metric metadata
	key := r.generateKey(point.Name, point.Dimensions)
	if metric, exists := r.metrics[key]; exists {
		metric.LastUpdated = point.Timestamp
		metric.Count++
	}
	
	// Store point
	if len(r.points) < r.maxPoints {
		r.points = append(r.points, point)
	} else {
		// Circular buffer - overwrite oldest
		r.points[len(r.points)%r.maxPoints] = point
	}
}

// GetMetricsForExport returns metrics ready for export
func (r *MetricsRegistry) GetMetricsForExport(batchSize int) []MetricPoint {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	if batchSize <= 0 || batchSize > len(r.points) {
		batchSize = len(r.points)
	}
	
	// Return the most recent metrics
	start := len(r.points) - batchSize
	if start < 0 {
		start = 0
	}
	
	result := make([]MetricPoint, batchSize)
	copy(result, r.points[start:])
	
	// Clear exported points
	r.points = r.points[:start]
	
	return result
}

// GetAllMetrics returns all metrics
func (r *MetricsRegistry) GetAllMetrics() []MetricPoint {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	result := make([]MetricPoint, len(r.points))
	copy(result, r.points)
	return result
}

// GetMetricCount returns the number of registered metrics
func (r *MetricsRegistry) GetMetricCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	return len(r.metrics)
}

// generateKey creates a unique key for a metric
func (r *MetricsRegistry) generateKey(name string, dimensions map[string]string) string {
	key := name
	
	// Sort dimension keys for consistent key generation
	keys := make([]string, 0, len(dimensions))
	for k := range dimensions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	
	for _, k := range keys {
		key += "_" + k + ":" + dimensions[k]
	}
	
	return key
}

// MetricsAggregator aggregates metrics over time windows
type MetricsAggregator struct {
	window      time.Duration
	metrics     map[string]*AggregatedMetric
	mu          sync.RWMutex
	lastReset   time.Time
}

// AggregatedMetric represents an aggregated metric
type AggregatedMetric struct {
	Name       string            `json:"name"`
	Count      int64             `json:"count"`
	Sum        float64           `json:"sum"`
	Min        float64           `json:"min"`
	Max        float64           `json:"max"`
	Mean       float64           `json:"mean"`
	StdDev     float64           `json:"std_dev"`
	Dimensions map[string]string `json:"dimensions"`
	Window     time.Duration     `json:"window"`
	Timestamp  time.Time         `json:"timestamp"`
}

// NewMetricsAggregator creates a new metrics aggregator
func NewMetricsAggregator() *MetricsAggregator {
	return NewMetricsAggregatorWithWindow(1 * time.Minute)
}

// NewMetricsAggregatorWithWindow creates an aggregator with a specific window
func NewMetricsAggregatorWithWindow(window time.Duration) *MetricsAggregator {
	return &MetricsAggregator{
		window:    window,
		metrics:   make(map[string]*AggregatedMetric),
		lastReset: time.Now(),
	}
}

// RecordValue records a value for aggregation
func (a *MetricsAggregator) RecordValue(name string, value float64, dimensions map[string]string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	key := a.generateKey(name, dimensions)
	
	metric, exists := a.metrics[key]
	if !exists {
		metric = &AggregatedMetric{
			Name:       name,
			Dimensions: dimensions,
			Window:     a.window,
			Min:        value,
			Max:        value,
			Timestamp:  time.Now(),
		}
		a.metrics[key] = metric
	}
	
	// Update aggregation
	metric.Count++
	metric.Sum += value
	metric.Mean = metric.Sum / float64(metric.Count)
	
	if value < metric.Min {
		metric.Min = value
	}
	if value > metric.Max {
		metric.Max = value
	}
	
	// Update standard deviation incrementally
	if metric.Count > 1 {
		oldMean := (metric.Sum - value) / float64(metric.Count-1)
		metric.StdDev = math.Sqrt(
			((float64(metric.Count-1) * metric.StdDev * metric.StdDev) + 
			(value-oldMean)*(value-metric.Mean)) / float64(metric.Count),
		)
	}
}

// Aggregate performs aggregation and returns results
func (a *MetricsAggregator) Aggregate() []*AggregatedMetric {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// Check if window has elapsed
	if time.Since(a.lastReset) < a.window {
		return nil
	}
	
	// Collect aggregated metrics
	results := make([]*AggregatedMetric, 0, len(a.metrics))
	for _, metric := range a.metrics {
		results = append(results, metric)
	}
	
	// Reset for next window
	a.metrics = make(map[string]*AggregatedMetric)
	a.lastReset = time.Now()
	
	return results
}

// generateKey creates a unique key for a metric
func (a *MetricsAggregator) generateKey(name string, dimensions map[string]string) string {
	key := name
	
	// Sort dimension keys for consistent key generation
	keys := make([]string, 0, len(dimensions))
	for k := range dimensions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	
	for _, k := range keys {
		key += "_" + k + ":" + dimensions[k]
	}
	
	return key
}

// GetAggregatedMetrics returns current aggregated metrics
func (a *MetricsAggregator) GetAggregatedMetrics() []*AggregatedMetric {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	results := make([]*AggregatedMetric, 0, len(a.metrics))
	for _, metric := range a.metrics {
		results = append(results, metric)
	}
	
	return results
}
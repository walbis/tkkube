package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// BackupMetrics holds all the backup-related metrics
type BackupMetrics struct {
	BackupDuration     prometheus.Histogram
	BackupErrors       prometheus.Counter
	ResourcesBackedUp  prometheus.Counter
	LastBackupTime     prometheus.Gauge
	NamespacesBackedUp prometheus.Gauge
}

// NewBackupMetrics creates a new set of backup metrics
func NewBackupMetrics() *BackupMetrics {
	return &BackupMetrics{
		BackupDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Name: "cluster_backup_duration_seconds",
			Help: "Duration of cluster backup operations in seconds",
			Buckets: []float64{1, 5, 10, 30, 60, 120, 300, 600, 1200}, // 1s to 20min
		}),
		BackupErrors: promauto.NewCounter(prometheus.CounterOpts{
			Name: "cluster_backup_errors_total",
			Help: "Total number of backup errors",
		}),
		ResourcesBackedUp: promauto.NewCounter(prometheus.CounterOpts{
			Name: "cluster_backup_resources_total",
			Help: "Total number of resources backed up",
		}),
		LastBackupTime: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "cluster_backup_last_success_timestamp",
			Help: "Timestamp of the last successful backup",
		}),
		NamespacesBackedUp: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "cluster_backup_namespaces_total",
			Help: "Number of namespaces backed up in the last operation",
		}),
	}
}

// Reset resets all metrics (useful for testing)
func (bm *BackupMetrics) Reset() {
	// Note: Prometheus metrics can't be reset easily, but we can provide this interface
	// for testing purposes. In production, metrics accumulate over time.
}
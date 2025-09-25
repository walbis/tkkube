# Production Simulation Suite

A comprehensive production-ready GitOps pipeline simulation with enterprise-grade backup, disaster recovery, monitoring, and validation capabilities.

## 🎯 Overview

This production simulation suite provides a complete end-to-end testing framework for GitOps pipelines, including:

- **Environment Setup**: Automated CRC cluster deployment with monitoring
- **Workload Simulation**: Realistic production applications (web, database, cache, workers)
- **Backup Integration**: Enterprise backup with MinIO S3-compatible storage
- **GitOps Pipeline**: Complete backup-to-GitOps artifact generation
- **Disaster Recovery**: Comprehensive DR simulation with monitoring
- **Validation Framework**: Real-time health and compliance monitoring
- **Test Orchestration**: Automated end-to-end testing with detailed reporting

## 🏗️ Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Environment   │    │    Workload     │    │     Backup      │
│     Setup       │───▶│   Deployment    │───▶│   Execution     │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         ▼                       ▼                       ▼
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│  Validation &   │    │  Disaster       │    │    GitOps       │
│  Monitoring     │    │  Recovery       │    │   Pipeline      │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         └───────────────────────┼───────────────────────┘
                                 ▼
                    ┌─────────────────┐
                    │ Master Test     │
                    │ Orchestrator    │
                    └─────────────────┘
```

## 📦 Components

### 1. Environment Setup (`environment-setup.sh`)
- Starts CRC (CodeReady Containers) cluster
- Deploys MinIO for S3-compatible backup storage
- Sets up monitoring namespace and basic observability
- Configures networking and storage for workloads

### 2. Workload Deployment (`deploy-workloads.sh`)
- Deploys realistic production applications:
  - Web application (nginx with custom content)
  - Database (PostgreSQL with sample data)
  - Cache layer (Redis with configuration)
  - Background workers (simulated job processing)
- Creates persistent volumes and ConfigMaps
- Implements health checks and resource limits

### 3. Backup Execution (`enhanced-backup-executor.go`)
- Go-based enterprise backup solution
- Extracts Kubernetes resources (deployments, services, ConfigMaps)
- Uploads backups to MinIO S3 storage
- Generates backup metadata and integrity checksums
- Supports incremental and full backup modes

### 4. GitOps Pipeline (`gitops-pipeline-orchestrator.sh`)
- Converts backups to GitOps artifacts
- Creates Kustomize base and overlay structure
- Generates ArgoCD and Flux configurations
- Implements multi-environment deployments (dev/staging/prod)
- Creates deployment pipelines with validation gates

### 5. Disaster Recovery (`disaster-recovery-simulator.sh`)
- Simulates various disaster scenarios:
  - Node failures and cluster instability
  - Namespace deletions and data loss
  - Network partitions and connectivity issues
  - Storage failures and data corruption
- Automated recovery procedures with monitoring
- Recovery time and success rate metrics

### 6. Validation Framework (`validation-monitoring-framework.go`)
- Real-time Kubernetes cluster health monitoring
- GitOps synchronization status validation
- Data integrity and backup consistency checks
- Security compliance monitoring (RBAC, network policies)
- Performance metrics collection and alerting
- HTTP endpoints for metrics and health checks

### 7. Master Orchestrator (`master-orchestrator.sh`)
- Coordinates execution of all simulation phases
- Provides comprehensive test reporting
- Implements parallel execution and dependency management
- Generates HTML dashboards and executive summaries
- Supports partial execution and failure recovery

## 🚀 Quick Start

### Prerequisites
- CRC (CodeReady Containers) installed and configured
- kubectl CLI tool
- Go 1.19+ for backup executor and validation framework
- jq for JSON processing
- curl for HTTP requests

### Basic Usage

1. **Run Complete Simulation**
   ```bash
   cd production-simulation
   ./master-orchestrator.sh run
   ```

2. **Run Individual Components**
   ```bash
   # Environment setup only
   ./environment-setup.sh
   
   # Deploy workloads
   ./deploy-workloads.sh
   
   # Execute backup
   go run enhanced-backup-executor.go
   
   # Generate GitOps artifacts
   ./gitops-pipeline-orchestrator.sh
   
   # Disaster recovery simulation
   ./disaster-recovery-simulator.sh
   
   # Start validation framework
   ./start-validation-framework.sh start
   ```

3. **Check Status and Reports**
   ```bash
   # Check orchestration status
   ./master-orchestrator.sh status
   
   # View available reports
   ./master-orchestrator.sh report
   
   # Clean up resources
   ./master-orchestrator.sh clean
   ```

## 📊 Monitoring and Metrics

### Validation Framework Endpoints
When the validation framework is running, access these endpoints:

- **Health Check**: `http://localhost:8080/health`
- **Prometheus Metrics**: `http://localhost:8080/metrics`
- **Validation Results**: `http://localhost:8080/validation-results`
- **Framework Status**: `http://localhost:8080/status`

### Key Metrics Tracked
- Cluster health and resource utilization
- Pod health and deployment status
- Backup success rates and integrity scores
- GitOps synchronization status
- Disaster recovery times and success rates
- Security compliance scores

## 🔧 Configuration

### Environment Variables
```bash
# Cluster configuration
CLUSTER_NAME="crc"
NAMESPACE="default"

# Backup configuration
BACKUP_LOCATION="./backup-source"
MINIO_ENDPOINT="localhost:9000"
MINIO_BUCKET="backups"

# GitOps configuration
GITOPS_REPO_PATH="./"
ARGOCD_NAMESPACE="argocd"
FLUX_NAMESPACE="flux-system"

# Monitoring configuration
METRICS_PORT="8080"
HEALTH_CHECK_INTERVAL="30s"

# Resource limits
MAX_CONCURRENT_VALIDATIONS="10"
VALIDATION_TIMEOUT="300s"
```

### Validation Framework Configuration
Edit `validation-config.yaml` to customize:
- Validation categories (Kubernetes, GitOps, Security)
- Performance thresholds
- Alert configurations
- Monitoring intervals

## 📝 Reports and Outputs

The simulation generates multiple types of reports:

### Executive Summary (`executive-summary.md`)
- High-level test results and success rates
- Phase breakdown with timings
- Key findings and recommendations

### HTML Dashboard (`orchestration-dashboard.html`)
- Interactive web dashboard with real-time status
- Visual progress bars and metrics
- Links to detailed logs and reports

### JSON Report (`test-orchestration-report-{timestamp}.json`)
- Machine-readable test results
- Detailed phase information and metrics
- Integration with CI/CD systems

### Individual Phase Logs
- `phase1-environment.log` - Environment setup details
- `phase2-workloads.log` - Workload deployment logs
- `phase3-backup.log` - Backup execution output
- `phase4-gitops.log` - GitOps pipeline generation
- `phase5-disaster-recovery.log` - DR simulation results
- `phase6-validation.log` - Validation framework output

## 🔍 Troubleshooting

### Common Issues

1. **CRC Cluster Not Starting**
   ```bash
   # Check CRC status
   crc status
   
   # Restart if needed
   crc stop && crc start
   ```

2. **MinIO Connection Issues**
   ```bash
   # Check MinIO pod status
   kubectl get pods -n minio-system
   
   # Port forward to access MinIO
   kubectl port-forward -n minio-system svc/minio 9000:9000
   ```

3. **Validation Framework Not Responding**
   ```bash
   # Check framework status
   ./start-validation-framework.sh status
   
   # Restart framework
   ./start-validation-framework.sh restart
   ```

4. **Go Module Issues**
   ```bash
   # Clean Go module cache
   go clean -modcache
   
   # Reinstall dependencies
   ./start-validation-framework.sh deps
   ```

### Debug Mode
Enable verbose logging by setting:
```bash
export DEBUG=true
export LOG_LEVEL=debug
```

## 🔐 Security Considerations

### Default Security Features
- RBAC policies for service accounts
- Network policies for pod communication
- Pod security standards enforcement
- Secret management for credentials
- TLS encryption for inter-service communication

### Security Testing
The framework includes automated security testing:
- Privileged container detection
- Service account configuration validation
- Network policy coverage analysis
- Certificate expiry monitoring
- RBAC policy compliance checks

## 📈 Performance Considerations

### Resource Requirements
- **Minimum**: 4 CPU cores, 8GB RAM, 50GB storage
- **Recommended**: 8 CPU cores, 16GB RAM, 100GB storage
- **Network**: Stable internet connection for container images

### Performance Tuning
- Adjust concurrent validation limits in config
- Configure resource quotas for workload namespaces
- Optimize backup intervals based on data change rate
- Use node affinity for performance-critical workloads

## 🤝 Contributing

### Development Setup
1. Fork the repository
2. Create feature branch
3. Make changes with tests
4. Run full simulation suite
5. Submit pull request with results

### Testing Guidelines
- All components must pass validation framework checks
- Include both positive and negative test cases
- Document configuration changes
- Provide performance impact analysis

## 📚 Additional Resources

### Documentation
- [Kubernetes Documentation](https://kubernetes.io/docs/)
- [ArgoCD Documentation](https://argo-cd.readthedocs.io/)
- [Flux Documentation](https://fluxcd.io/docs/)
- [CRC Documentation](https://crc.dev/crc/)

### Related Projects
- [Kustomize](https://kustomize.io/) - Kubernetes configuration management
- [MinIO](https://min.io/) - S3-compatible object storage
- [Prometheus](https://prometheus.io/) - Monitoring and alerting

## 📞 Support

For issues, questions, or contributions:
- Create GitHub issue for bugs or feature requests
- Review existing documentation and troubleshooting guides
- Check logs in the results directory for detailed error information

---

**Version**: 1.0.0  
**Last Updated**: $(date)  
**Compatibility**: Kubernetes 1.24+, CRC 2.0+
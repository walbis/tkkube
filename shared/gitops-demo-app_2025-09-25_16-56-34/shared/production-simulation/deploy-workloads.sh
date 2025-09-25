#!/bin/bash
# Deploy Realistic Production Workloads for Backup Testing
# Simulates real-world applications with various Kubernetes resource types

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SIMULATION_NAMESPACE="production-simulation"

log() {
    echo -e "${BLUE}[$(date +'%Y-%m-%d %H:%M:%S')]${NC} $1"
}

success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

deploy_web_application() {
    log "Deploying web application stack..."
    
    cat > "$SCRIPT_DIR/web-application.yaml" << 'EOF'
# Multi-tier Web Application
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: web-app-config
  namespace: production-simulation
  labels:
    app: web-application
    tier: configuration
data:
  app.properties: |
    server.port=8080
    database.host=postgres-service
    database.port=5432
    database.name=webapp_db
    redis.host=redis-service
    redis.port=6379
    log.level=INFO
    environment=production
    feature.analytics.enabled=true
    feature.cache.enabled=true
  nginx.conf: |
    server {
        listen 80;
        server_name localhost;
        
        location / {
            proxy_pass http://backend-service:8080;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
        }
        
        location /static/ {
            alias /usr/share/nginx/html/static/;
            expires 30d;
        }
        
        location /health {
            access_log off;
            return 200 "healthy\n";
        }
    }
---
apiVersion: v1
kind: Secret
metadata:
  name: web-app-secrets
  namespace: production-simulation
  labels:
    app: web-application
    tier: configuration
type: Opaque
data:
  database-username: d2ViYXBwX3VzZXI=  # webapp_user
  database-password: c2VjdXJlUGFzcw==  # securePass
  jwt-secret: c3VwZXJfc2VjcmV0X2p3dF9rZXk=  # super_secret_jwt_key
  api-key: YWJjZGVmZ2hpams=  # abcdefghijk
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-frontend
  namespace: production-simulation
  labels:
    app: web-application
    tier: frontend
spec:
  replicas: 3
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 1
  selector:
    matchLabels:
      app: web-application
      tier: frontend
  template:
    metadata:
      labels:
        app: web-application
        tier: frontend
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "80"
        prometheus.io/path: "/metrics"
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 101
        fsGroup: 101
      containers:
      - name: nginx
        image: nginx:1.24-alpine
        ports:
        - containerPort: 80
          name: http
        env:
        - name: BACKEND_HOST
          value: "backend-service"
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 300m
            memory: 256Mi
        volumeMounts:
        - name: nginx-config
          mountPath: /etc/nginx/conf.d
          readOnly: true
        - name: static-content
          mountPath: /usr/share/nginx/html/static
          readOnly: true
        livenessProbe:
          httpGet:
            path: /health
            port: 80
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: 80
          initialDelaySeconds: 5
          periodSeconds: 5
      volumes:
      - name: nginx-config
        configMap:
          name: web-app-config
          items:
          - key: nginx.conf
            path: default.conf
      - name: static-content
        emptyDir: {}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-backend
  namespace: production-simulation
  labels:
    app: web-application
    tier: backend
spec:
  replicas: 2
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0
  selector:
    matchLabels:
      app: web-application
      tier: backend
  template:
    metadata:
      labels:
        app: web-application
        tier: backend
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8080"
        prometheus.io/path: "/actuator/prometheus"
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        fsGroup: 1000
      containers:
      - name: backend
        image: openjdk:11-jre-slim
        command: ["java"]
        args: ["-jar", "/app/application.jar"]
        ports:
        - containerPort: 8080
          name: http
        env:
        - name: DATABASE_HOST
          valueFrom:
            configMapKeyRef:
              name: web-app-config
              key: app.properties
        - name: DATABASE_USERNAME
          valueFrom:
            secretKeyRef:
              name: web-app-secrets
              key: database-username
        - name: DATABASE_PASSWORD
          valueFrom:
            secretKeyRef:
              name: web-app-secrets
              key: database-password
        - name: JWT_SECRET
          valueFrom:
            secretKeyRef:
              name: web-app-secrets
              key: jwt-secret
        resources:
          requests:
            cpu: 200m
            memory: 512Mi
          limits:
            cpu: 1000m
            memory: 1Gi
        volumeMounts:
        - name: app-config
          mountPath: /app/config
          readOnly: true
        - name: app-logs
          mountPath: /app/logs
        livenessProbe:
          httpGet:
            path: /actuator/health
            port: 8080
          initialDelaySeconds: 60
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /actuator/health/readiness
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
      volumes:
      - name: app-config
        configMap:
          name: web-app-config
      - name: app-logs
        emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: frontend-service
  namespace: production-simulation
  labels:
    app: web-application
    tier: frontend
spec:
  type: ClusterIP
  ports:
  - port: 80
    targetPort: 80
    name: http
  selector:
    app: web-application
    tier: frontend
---
apiVersion: v1
kind: Service
metadata:
  name: backend-service
  namespace: production-simulation
  labels:
    app: web-application
    tier: backend
spec:
  type: ClusterIP
  ports:
  - port: 8080
    targetPort: 8080
    name: http
  selector:
    app: web-application
    tier: backend
EOF

    oc apply -f "$SCRIPT_DIR/web-application.yaml"
    success "Web application deployed"
}

deploy_database_stack() {
    log "Deploying database stack with persistent storage..."
    
    cat > "$SCRIPT_DIR/database-stack.yaml" << 'EOF'
# PostgreSQL Database with Persistent Storage
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: postgres-config
  namespace: production-simulation
  labels:
    app: postgres
    tier: database
data:
  postgresql.conf: |
    # PostgreSQL Configuration
    max_connections = 100
    shared_buffers = 128MB
    effective_cache_size = 256MB
    work_mem = 4MB
    maintenance_work_mem = 64MB
    checkpoint_completion_target = 0.9
    wal_buffers = 16MB
    default_statistics_target = 100
    random_page_cost = 1.1
    effective_io_concurrency = 200
    log_min_duration_statement = 1000
    log_checkpoints = on
    log_connections = on
    log_disconnections = on
    log_lock_waits = on
---
apiVersion: v1
kind: Secret
metadata:
  name: postgres-secrets
  namespace: production-simulation
  labels:
    app: postgres
    tier: database
type: Opaque
data:
  postgres-password: cG9zdGdyZXNfcGFzcw==  # postgres_pass
  webapp-username: d2ViYXBwX3VzZXI=  # webapp_user
  webapp-password: c2VjdXJlUGFzcw==  # securePass
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: postgres-storage
  namespace: production-simulation
  labels:
    app: postgres
    tier: database
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 5Gi
  storageClassName: crc-csi-hostpath-provisioner
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: postgres
  namespace: production-simulation
  labels:
    app: postgres
    tier: database
spec:
  replicas: 1
  strategy:
    type: Recreate
  selector:
    matchLabels:
      app: postgres
      tier: database
  template:
    metadata:
      labels:
        app: postgres
        tier: database
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "9187"
        prometheus.io/path: "/metrics"
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 999
        fsGroup: 999
      containers:
      - name: postgres
        image: postgres:15-alpine
        ports:
        - containerPort: 5432
          name: postgres
        env:
        - name: POSTGRES_DB
          value: "webapp_db"
        - name: POSTGRES_USER
          value: "postgres"
        - name: POSTGRES_PASSWORD
          valueFrom:
            secretKeyRef:
              name: postgres-secrets
              key: postgres-password
        - name: PGDATA
          value: /var/lib/postgresql/data/pgdata
        resources:
          requests:
            cpu: 200m
            memory: 256Mi
          limits:
            cpu: 500m
            memory: 512Mi
        volumeMounts:
        - name: postgres-storage
          mountPath: /var/lib/postgresql/data
        - name: postgres-config
          mountPath: /etc/postgresql
          readOnly: true
        - name: init-scripts
          mountPath: /docker-entrypoint-initdb.d
          readOnly: true
        livenessProbe:
          exec:
            command:
            - pg_isready
            - -U
            - postgres
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          exec:
            command:
            - pg_isready
            - -U
            - postgres
          initialDelaySeconds: 5
          periodSeconds: 5
      - name: postgres-exporter
        image: prometheuscommunity/postgres-exporter:latest
        ports:
        - containerPort: 9187
          name: metrics
        env:
        - name: DATA_SOURCE_NAME
          value: "postgresql://postgres:postgres_pass@localhost:5432/webapp_db?sslmode=disable"
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: 100m
            memory: 128Mi
      volumes:
      - name: postgres-storage
        persistentVolumeClaim:
          claimName: postgres-storage
      - name: postgres-config
        configMap:
          name: postgres-config
      - name: init-scripts
        configMap:
          name: postgres-init-scripts
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: postgres-init-scripts
  namespace: production-simulation
  labels:
    app: postgres
    tier: database
data:
  01-init-webapp.sql: |
    -- Initialize webapp database
    CREATE USER webapp_user WITH PASSWORD 'securePass';
    CREATE DATABASE webapp_db OWNER webapp_user;
    GRANT ALL PRIVILEGES ON DATABASE webapp_db TO webapp_user;
    
    \c webapp_db;
    
    -- Create application tables
    CREATE TABLE users (
        id SERIAL PRIMARY KEY,
        username VARCHAR(255) UNIQUE NOT NULL,
        email VARCHAR(255) UNIQUE NOT NULL,
        password_hash VARCHAR(255) NOT NULL,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );
    
    CREATE TABLE sessions (
        id SERIAL PRIMARY KEY,
        user_id INTEGER REFERENCES users(id),
        session_token VARCHAR(255) UNIQUE NOT NULL,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        expires_at TIMESTAMP NOT NULL
    );
    
    CREATE TABLE audit_log (
        id SERIAL PRIMARY KEY,
        user_id INTEGER REFERENCES users(id),
        action VARCHAR(255) NOT NULL,
        resource VARCHAR(255),
        timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        ip_address INET,
        user_agent TEXT
    );
    
    -- Insert sample data
    INSERT INTO users (username, email, password_hash) VALUES
    ('admin', 'admin@localhost', '$2b$12$example_hash_1'),
    ('user1', 'user1@localhost', '$2b$12$example_hash_2'),
    ('user2', 'user2@localhost', '$2b$12$example_hash_3');
    
    -- Grant permissions
    GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO webapp_user;
    GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO webapp_user;
---
apiVersion: v1
kind: Service
metadata:
  name: postgres-service
  namespace: production-simulation
  labels:
    app: postgres
    tier: database
spec:
  type: ClusterIP
  ports:
  - port: 5432
    targetPort: 5432
    name: postgres
  - port: 9187
    targetPort: 9187
    name: metrics
  selector:
    app: postgres
    tier: database
EOF

    oc apply -f "$SCRIPT_DIR/database-stack.yaml"
    success "Database stack deployed"
}

deploy_cache_layer() {
    log "Deploying Redis cache layer..."
    
    cat > "$SCRIPT_DIR/cache-layer.yaml" << 'EOF'
# Redis Cache Layer
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: redis-config
  namespace: production-simulation
  labels:
    app: redis
    tier: cache
data:
  redis.conf: |
    # Redis Configuration
    maxmemory 256mb
    maxmemory-policy allkeys-lru
    timeout 300
    tcp-keepalive 60
    databases 16
    save 900 1
    save 300 10
    save 60 10000
    rdbcompression yes
    rdbchecksum yes
    dbfilename dump.rdb
    dir /data
    appendonly yes
    appendfsync everysec
    no-appendfsync-on-rewrite no
    auto-aof-rewrite-percentage 100
    auto-aof-rewrite-min-size 64mb
---
apiVersion: v1
kind: Secret
metadata:
  name: redis-secrets
  namespace: production-simulation
  labels:
    app: redis
    tier: cache
type: Opaque
data:
  redis-password: cmVkaXNfcGFzcw==  # redis_pass
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: redis-storage
  namespace: production-simulation
  labels:
    app: redis
    tier: cache
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: crc-csi-hostpath-provisioner
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis
  namespace: production-simulation
  labels:
    app: redis
    tier: cache
spec:
  replicas: 1
  strategy:
    type: Recreate
  selector:
    matchLabels:
      app: redis
      tier: cache
  template:
    metadata:
      labels:
        app: redis
        tier: cache
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "9121"
        prometheus.io/path: "/metrics"
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 999
        fsGroup: 999
      containers:
      - name: redis
        image: redis:7-alpine
        command:
          - redis-server
          - /etc/redis/redis.conf
        ports:
        - containerPort: 6379
          name: redis
        env:
        - name: REDIS_PASSWORD
          valueFrom:
            secretKeyRef:
              name: redis-secrets
              key: redis-password
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 300m
            memory: 256Mi
        volumeMounts:
        - name: redis-storage
          mountPath: /data
        - name: redis-config
          mountPath: /etc/redis
          readOnly: true
        livenessProbe:
          exec:
            command:
            - redis-cli
            - ping
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          exec:
            command:
            - redis-cli
            - ping
          initialDelaySeconds: 5
          periodSeconds: 5
      - name: redis-exporter
        image: oliver006/redis_exporter:latest
        ports:
        - containerPort: 9121
          name: metrics
        env:
        - name: REDIS_ADDR
          value: "localhost:6379"
        - name: REDIS_PASSWORD
          valueFrom:
            secretKeyRef:
              name: redis-secrets
              key: redis-password
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: 100m
            memory: 128Mi
      volumes:
      - name: redis-storage
        persistentVolumeClaim:
          claimName: redis-storage
      - name: redis-config
        configMap:
          name: redis-config
---
apiVersion: v1
kind: Service
metadata:
  name: redis-service
  namespace: production-simulation
  labels:
    app: redis
    tier: cache
spec:
  type: ClusterIP
  ports:
  - port: 6379
    targetPort: 6379
    name: redis
  - port: 9121
    targetPort: 9121
    name: metrics
  selector:
    app: redis
    tier: cache
EOF

    oc apply -f "$SCRIPT_DIR/cache-layer.yaml"
    success "Cache layer deployed"
}

deploy_worker_services() {
    log "Deploying background worker services..."
    
    cat > "$SCRIPT_DIR/worker-services.yaml" << 'EOF'
# Background Worker Services
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: worker-config
  namespace: production-simulation
  labels:
    app: workers
    tier: processing
data:
  worker.properties: |
    queue.host=rabbitmq-service
    queue.port=5672
    queue.vhost=/
    database.host=postgres-service
    database.port=5432
    database.name=webapp_db
    redis.host=redis-service
    redis.port=6379
    worker.threads=4
    batch.size=10
    retry.attempts=3
    log.level=INFO
---
apiVersion: v1
kind: Secret
metadata:
  name: worker-secrets
  namespace: production-simulation
  labels:
    app: workers
    tier: processing
type: Opaque
data:
  queue-username: d29ya2Vy  # worker
  queue-password: d29ya2VyX3Bhc3M=  # worker_pass
  database-username: d2ViYXBwX3VzZXI=  # webapp_user
  database-password: c2VjdXJlUGFzcw==  # securePass
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: email-worker
  namespace: production-simulation
  labels:
    app: workers
    tier: processing
    component: email
spec:
  replicas: 2
  selector:
    matchLabels:
      app: workers
      tier: processing
      component: email
  template:
    metadata:
      labels:
        app: workers
        tier: processing
        component: email
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8080"
        prometheus.io/path: "/metrics"
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        fsGroup: 1000
      containers:
      - name: email-worker
        image: python:3.9-slim
        command: ["python"]
        args: ["/app/email_worker.py"]
        env:
        - name: WORKER_TYPE
          value: "email"
        - name: QUEUE_HOST
          value: "rabbitmq-service"
        - name: QUEUE_USERNAME
          valueFrom:
            secretKeyRef:
              name: worker-secrets
              key: queue-username
        - name: QUEUE_PASSWORD
          valueFrom:
            secretKeyRef:
              name: worker-secrets
              key: queue-password
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 300m
            memory: 256Mi
        volumeMounts:
        - name: worker-config
          mountPath: /app/config
          readOnly: true
        - name: worker-logs
          mountPath: /app/logs
      volumes:
      - name: worker-config
        configMap:
          name: worker-config
      - name: worker-logs
        emptyDir: {}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: analytics-worker
  namespace: production-simulation
  labels:
    app: workers
    tier: processing
    component: analytics
spec:
  replicas: 1
  selector:
    matchLabels:
      app: workers
      tier: processing
      component: analytics
  template:
    metadata:
      labels:
        app: workers
        tier: processing
        component: analytics
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8080"
        prometheus.io/path: "/metrics"
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        fsGroup: 1000
      containers:
      - name: analytics-worker
        image: python:3.9-slim
        command: ["python"]
        args: ["/app/analytics_worker.py"]
        env:
        - name: WORKER_TYPE
          value: "analytics"
        - name: DATABASE_HOST
          value: "postgres-service"
        - name: DATABASE_USERNAME
          valueFrom:
            secretKeyRef:
              name: worker-secrets
              key: database-username
        - name: DATABASE_PASSWORD
          valueFrom:
            secretKeyRef:
              name: worker-secrets
              key: database-password
        - name: REDIS_HOST
          value: "redis-service"
        resources:
          requests:
            cpu: 200m
            memory: 256Mi
          limits:
            cpu: 500m
            memory: 512Mi
        volumeMounts:
        - name: worker-config
          mountPath: /app/config
          readOnly: true
        - name: worker-data
          mountPath: /app/data
      volumes:
      - name: worker-config
        configMap:
          name: worker-config
      - name: worker-data
        emptyDir: {}
EOF

    oc apply -f "$SCRIPT_DIR/worker-services.yaml"
    success "Worker services deployed"
}

create_network_policies() {
    log "Creating network policies for security..."
    
    cat > "$SCRIPT_DIR/network-policies.yaml" << 'EOF'
# Network Policies for Production Simulation
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: frontend-network-policy
  namespace: production-simulation
spec:
  podSelector:
    matchLabels:
      app: web-application
      tier: frontend
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from: []
    ports:
    - protocol: TCP
      port: 80
  egress:
  - to:
    - podSelector:
        matchLabels:
          app: web-application
          tier: backend
    ports:
    - protocol: TCP
      port: 8080
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: backend-network-policy
  namespace: production-simulation
spec:
  podSelector:
    matchLabels:
      app: web-application
      tier: backend
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - podSelector:
        matchLabels:
          app: web-application
          tier: frontend
    ports:
    - protocol: TCP
      port: 8080
  egress:
  - to:
    - podSelector:
        matchLabels:
          app: postgres
          tier: database
    ports:
    - protocol: TCP
      port: 5432
  - to:
    - podSelector:
        matchLabels:
          app: redis
          tier: cache
    ports:
    - protocol: TCP
      port: 6379
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: database-network-policy
  namespace: production-simulation
spec:
  podSelector:
    matchLabels:
      app: postgres
      tier: database
  policyTypes:
  - Ingress
  ingress:
  - from:
    - podSelector:
        matchLabels:
          app: web-application
          tier: backend
    - podSelector:
        matchLabels:
          app: workers
          tier: processing
    ports:
    - protocol: TCP
      port: 5432
EOF

    oc apply -f "$SCRIPT_DIR/network-policies.yaml"
    success "Network policies created"
}

wait_for_deployments() {
    log "Waiting for all deployments to be ready..."
    
    local deployments=(
        "web-frontend"
        "web-backend"
        "postgres"
        "redis"
        "email-worker"
        "analytics-worker"
    )
    
    for deployment in "${deployments[@]}"; do
        log "Waiting for $deployment to be ready..."
        oc wait --for=condition=available deployment/"$deployment" \
            -n "$SIMULATION_NAMESPACE" --timeout=300s
    done
    
    success "All deployments are ready"
}

create_ingress_routes() {
    log "Creating OpenShift routes for external access..."
    
    cat > "$SCRIPT_DIR/routes.yaml" << 'EOF'
# OpenShift Routes for External Access
---
apiVersion: route.openshift.io/v1
kind: Route
metadata:
  name: web-frontend-route
  namespace: production-simulation
  labels:
    app: web-application
    tier: frontend
spec:
  to:
    kind: Service
    name: frontend-service
    weight: 100
  port:
    targetPort: http
  wildcardPolicy: None
---
apiVersion: route.openshift.io/v1
kind: Route
metadata:
  name: web-backend-api-route
  namespace: production-simulation
  labels:
    app: web-application
    tier: backend
spec:
  path: /api
  to:
    kind: Service
    name: backend-service
    weight: 100
  port:
    targetPort: http
  wildcardPolicy: None
EOF

    oc apply -f "$SCRIPT_DIR/routes.yaml"
    success "Routes created"
}

main() {
    log "Deploying Realistic Production Workloads"
    log "========================================"
    
    # Check if namespace exists
    if ! oc get namespace "$SIMULATION_NAMESPACE" >/dev/null 2>&1; then
        error "Simulation namespace '$SIMULATION_NAMESPACE' does not exist. Run environment-setup.sh first."
    fi
    
    # Deploy all components
    deploy_web_application
    deploy_database_stack
    deploy_cache_layer
    deploy_worker_services
    create_network_policies
    create_ingress_routes
    
    # Wait for everything to be ready
    wait_for_deployments
    
    success "All production workloads deployed successfully!"
    
    # Get route information
    log "Getting route information..."
    FRONTEND_ROUTE=$(oc get route web-frontend-route -n "$SIMULATION_NAMESPACE" -o jsonpath='{.spec.host}' 2>/dev/null || echo "Route not ready")
    BACKEND_ROUTE=$(oc get route web-backend-api-route -n "$SIMULATION_NAMESPACE" -o jsonpath='{.spec.host}' 2>/dev/null || echo "Route not ready")
    
    cat << EOF

=== DEPLOYMENT SUMMARY ===
✅ Web Application (Frontend + Backend): 5 replicas total
✅ PostgreSQL Database: 1 replica with 5Gi storage
✅ Redis Cache: 1 replica with 1Gi storage  
✅ Background Workers: 3 replicas total
✅ Network Policies: Security isolation implemented
✅ Routes: External access configured

=== DEPLOYED RESOURCES ===
Deployments: $(oc get deployments -n $SIMULATION_NAMESPACE --no-headers | wc -l)
Services: $(oc get services -n $SIMULATION_NAMESPACE --no-headers | wc -l)
ConfigMaps: $(oc get configmaps -n $SIMULATION_NAMESPACE --no-headers | wc -l)
Secrets: $(oc get secrets -n $SIMULATION_NAMESPACE --no-headers | wc -l)
PersistentVolumeClaims: $(oc get pvc -n $SIMULATION_NAMESPACE --no-headers | wc -l)
NetworkPolicies: $(oc get networkpolicies -n $SIMULATION_NAMESPACE --no-headers | wc -l)
Routes: $(oc get routes -n $SIMULATION_NAMESPACE --no-headers | wc -l)

=== ACCESS URLS ===
Frontend: http://$FRONTEND_ROUTE
Backend API: http://$BACKEND_ROUTE

=== MONITORING ===
Pod Status: oc get pods -n $SIMULATION_NAMESPACE
Resource Usage: oc top pods -n $SIMULATION_NAMESPACE
Logs: oc logs -f deployment/<deployment-name> -n $SIMULATION_NAMESPACE

=== NEXT STEPS ===
1. Verify application functionality
2. Generate some test data/traffic
3. Execute backup: ./backup-executor.sh $SIMULATION_NAMESPACE
4. Test GitOps pipeline: ./generate-gitops.sh

EOF
}

main "$@"
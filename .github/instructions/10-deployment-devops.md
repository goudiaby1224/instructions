# Deployment & DevOps for Go APIs

## Docker Configuration

### Multi-stage Dockerfile
```dockerfile
# Build stage
FROM golang:1.21-alpine AS builder

# Install dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Create non-root user
RUN adduser -D -g '' appuser

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.Version=$(git describe --tags --always --dirty) -X main.BuildTime=$(date -u +'%Y-%m-%dT%H:%M:%SZ')" \
    -o api ./cmd/api

# Final stage
FROM scratch

# Import from builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/passwd /etc/passwd

# Copy the binary
COPY --from=builder /build/api /api

# Copy migrations if needed
COPY --from=builder /build/migrations /migrations

# Use non-root user
USER appuser

EXPOSE 8080

ENTRYPOINT ["/api"]
```

### Docker Compose for Development
```yaml
# docker-compose.yml
version: '3.8'

services:
  api:
    build:
      context: .
      dockerfile: Dockerfile
    ports:
      - "8080:8080"
    environment:
      - DB_HOST=postgres
      - DB_PORT=5432
      - DB_USER=apiuser
      - DB_PASSWORD=apipass
      - DB_NAME=apidb
      - REDIS_HOST=redis
      - REDIS_PORT=6379
    depends_on:
      - postgres
      - redis
    volumes:
      - ./configs:/configs:ro
    networks:
      - api-network

  postgres:
    image: postgres:15-alpine
    ports:
      - "5432:5432"
    environment:
      - POSTGRES_USER=apiuser
      - POSTGRES_PASSWORD=apipass
      - POSTGRES_DB=apidb
    volumes:
      - postgres-data:/var/lib/postgresql/data
    networks:
      - api-network

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    networks:
      - api-network

  prometheus:
    image: prom/prometheus:latest
    ports:
      - "9090:9090"
    volumes:
      - ./configs/prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - prometheus-data:/prometheus
    networks:
      - api-network

  grafana:
    image: grafana/grafana:latest
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
    volumes:
      - grafana-data:/var/lib/grafana
    networks:
      - api-network

volumes:
  postgres-data:
  prometheus-data:
  grafana-data:

networks:
  api-network:
    driver: bridge
```

## Kubernetes Deployment

### Deployment Configuration
```yaml
# k8s/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api-deployment
  namespace: production
  labels:
    app: api
spec:
  replicas: 3
  selector:
    matchLabels:
      app: api
  template:
    metadata:
      labels:
        app: api
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8080"
        prometheus.io/path: "/metrics"
    spec:
      serviceAccountName: api-service-account
      containers:
      - name: api
        image: myregistry/api:v1.0.0
        ports:
        - containerPort: 8080
          name: http
        env:
        - name: DB_HOST
          valueFrom:
            secretKeyRef:
              name: api-secrets
              key: db-host
        - name: DB_PASSWORD
          valueFrom:
            secretKeyRef:
              name: api-secrets
              key: db-password
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
        volumeMounts:
        - name: config
          mountPath: /etc/api
          readOnly: true
      volumes:
      - name: config
        configMap:
          name: api-config
```

### Service Configuration
```yaml
# k8s/service.yaml
apiVersion: v1
kind: Service
metadata:
  name: api-service
  namespace: production
  labels:
    app: api
spec:
  type: ClusterIP
  ports:
  - port: 80
    targetPort: 8080
    protocol: TCP
    name: http
  selector:
    app: api

---
apiVersion: v1
kind: Service
metadata:
  name: api-service-nodeport
  namespace: production
spec:
  type: NodePort
  ports:
  - port: 80
    targetPort: 8080
    nodePort: 30080
  selector:
    app: api
```

### Horizontal Pod Autoscaler
```yaml
# k8s/hpa.yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: api-hpa
  namespace: production
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: api-deployment
  minReplicas: 2
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
  - type: Pods
    pods:
      metric:
        name: http_requests_per_second
      target:
        type: AverageValue
        averageValue: "1000"
```

## CI/CD Pipeline

### GitHub Actions
```yaml
# .github/workflows/ci-cd.yml
name: CI/CD Pipeline

on:
  push:
    branches: [main, develop]
  pull_request:
    branches: [main]

env:
  GO_VERSION: '1.21'
  DOCKER_REGISTRY: ghcr.io

jobs:
  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:15
        env:
          POSTGRES_PASSWORD: test
          POSTGRES_DB: testdb
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
        ports:
          - 5432:5432

    steps:
    - uses: actions/checkout@v3
    
    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: ${{ env.GO_VERSION }}
    
    - name: Cache Go modules
      uses: actions/cache@v3
      with:
        path: ~/go/pkg/mod
        key: ${{ runner.os }}-go-${{ hashFiles('**/go.sum') }}
        restore-keys: |
          ${{ runner.os }}-go-
    
    - name: Install dependencies
      run: go mod download
    
    - name: Run tests
      run: |
        go test -v -race -coverprofile=coverage.out ./...
        go tool cover -html=coverage.out -o coverage.html
    
    - name: Run linters
      uses: golangci/golangci-lint-action@v3
      with:
        version: latest
    
    - name: Upload coverage
      uses: codecov/codecov-action@v3
      with:
        file: ./coverage.out

  build:
    needs: test
    runs-on: ubuntu-latest
    if: github.event_name == 'push'
    
    steps:
    - uses: actions/checkout@v3
    
    - name: Set up Docker Buildx
      uses: docker/setup-buildx-action@v2
    
    - name: Log in to registry
      uses: docker/login-action@v2
      with:
        registry: ${{ env.DOCKER_REGISTRY }}
        username: ${{ github.actor }}
        password: ${{ secrets.GITHUB_TOKEN }}
    
    - name: Extract metadata
      id: meta
      uses: docker/metadata-action@v4
      with:
        images: ${{ env.DOCKER_REGISTRY }}/${{ github.repository }}
        tags: |
          type=ref,event=branch
          type=ref,event=pr
          type=semver,pattern={{version}}
          type=semver,pattern={{major}}.{{minor}}
          type=sha
    
    - name: Build and push
      uses: docker/build-push-action@v4
      with:
        context: .
        push: true
        tags: ${{ steps.meta.outputs.tags }}
        labels: ${{ steps.meta.outputs.labels }}
        cache-from: type=gha
        cache-to: type=gha,mode=max

  deploy:
    needs: build
    runs-on: ubuntu-latest
    if: github.ref == 'refs/heads/main'
    
    steps:
    - name: Deploy to Kubernetes
      run: |
        # Configure kubectl
        echo "${{ secrets.KUBECONFIG }}" | base64 -d > kubeconfig
        export KUBECONFIG=kubeconfig
        
        # Update deployment
        kubectl set image deployment/api-deployment \
          api=${{ env.DOCKER_REGISTRY }}/${{ github.repository }}:sha-${GITHUB_SHA:0:7} \
          -n production
        
        # Wait for rollout
        kubectl rollout status deployment/api-deployment -n production
```

## Monitoring and Observability

### Health Check Implementation
```go
// internal/health/health.go
package health

import (
    "context"
    "encoding/json"
    "net/http"
    "sync"
    "time"
)

type Checker interface {
    Check(ctx context.Context) error
}

type Health struct {
    checkers map[string]Checker
    mu       sync.RWMutex
}

func New() *Health {
    return &Health{
        checkers: make(map[string]Checker),
    }
}

func (h *Health) Register(name string, checker Checker) {
    h.mu.Lock()
    defer h.mu.Unlock()
    h.checkers[name] = checker
}

func (h *Health) Handler() http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
        defer cancel()
        
        status := h.Check(ctx)
        
        w.Header().Set("Content-Type", "application/json")
        if !status.Healthy {
            w.WriteHeader(http.StatusServiceUnavailable)
        }
        
        json.NewEncoder(w).Encode(status)
    }
}

func (h *Health) Check(ctx context.Context) HealthStatus {
    h.mu.RLock()
    defer h.mu.RUnlock()
    
    status := HealthStatus{
        Healthy: true,
        Checks:  make(map[string]CheckResult),
    }
    
    var wg sync.WaitGroup
    var mu sync.Mutex
    
    for name, checker := range h.checkers {
        wg.Add(1)
        go func(name string, checker Checker) {
            defer wg.Done()
            
            result := CheckResult{Healthy: true}
            start := time.Now()
            
            if err := checker.Check(ctx); err != nil {
                result.Healthy = false
                result.Error = err.Error()
            }
            
            result.Duration = time.Since(start).String()
            
            mu.Lock()
            status.Checks[name] = result
            if !result.Healthy {
                status.Healthy = false
            }
            mu.Unlock()
        }(name, checker)
    }
    
    wg.Wait()
    return status
}

// Database health check
type DatabaseChecker struct {
    db *sql.DB
}

func (c *DatabaseChecker) Check(ctx context.Context) error {
    return c.db.PingContext(ctx)
}

// Redis health check
type RedisChecker struct {
    client *redis.Client
}

func (c *RedisChecker) Check(ctx context.Context) error {
    return c.client.Ping(ctx).Err()
}
```

### Structured Logging
```go
// internal/logger/logger.go
package logger

import (
    "os"
    
    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
)

func NewProduction() (*zap.Logger, error) {
    config := zap.NewProductionConfig()
    
    // Configure based on environment
    if os.Getenv("LOG_LEVEL") != "" {
        level, err := zapcore.ParseLevel(os.Getenv("LOG_LEVEL"))
        if err != nil {
            return nil, err
        }
        config.Level = zap.NewAtomicLevelAt(level)
    }
    
    // Add custom fields
    config.InitialFields = map[string]interface{}{
        "service": os.Getenv("SERVICE_NAME"),
        "version": os.Getenv("SERVICE_VERSION"),
    }
    
    // Configure output format
    if os.Getenv("LOG_FORMAT") == "json" {
        config.Encoding = "json"
    }
    
    return config.Build()
}

// Request logging middleware
func LoggingMiddleware(logger *zap.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()
            
            wrapped := &responseWriter{
                ResponseWriter: w,
                statusCode:     http.StatusOK,
            }
            
            next.ServeHTTP(wrapped, r)
            
            logger.Info("http_request",
                zap.String("method", r.Method),
                zap.String("path", r.URL.Path),
                zap.Int("status", wrapped.statusCode),
                zap.Duration("duration", time.Since(start)),
                zap.String("request_id", GetRequestID(r.Context())),
                zap.String("user_agent", r.UserAgent()),
                zap.String("remote_addr", r.RemoteAddr),
            )
        })
    }
}
```

### Distributed Tracing
```go
// internal/tracing/tracing.go
package tracing

import (
    "io"
    
    "github.com/opentracing/opentracing-go"
    "github.com/uber/jaeger-client-go"
    jaegercfg "github.com/uber/jaeger-client-go/config"
)

func InitJaeger(service string) (opentracing.Tracer, io.Closer, error) {
    cfg := jaegercfg.Configuration{
        ServiceName: service,
        Sampler: &jaegercfg.SamplerConfig{
            Type:  jaeger.SamplerTypeConst,
            Param: 1,
        },
        Reporter: &jaegercfg.ReporterConfig{
            LogSpans: true,
        },
    }
    
    tracer, closer, err := cfg.NewTracer(
        jaegercfg.Logger(jaeger.StdLogger),
    )
    
    if err != nil {
        return nil, nil, err
    }
    
    opentracing.SetGlobalTracer(tracer)
    
    return tracer, closer, nil
}

// Tracing middleware
func TracingMiddleware(tracer opentracing.Tracer) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            spanCtx, _ := tracer.Extract(
                opentracing.HTTPHeaders,
                opentracing.HTTPHeadersCarrier(r.Header),
            )
            
            span := tracer.StartSpan(
                fmt.Sprintf("%s %s", r.Method, r.URL.Path),
                ext.RPCServerOption(spanCtx),
            )
            defer span.Finish()
            
            ext.HTTPMethod.Set(span, r.Method)
            ext.HTTPUrl.Set(span, r.URL.String())
            
            ctx := opentracing.ContextWithSpan(r.Context(), span)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

## Security Best Practices

### Security Headers Middleware
```go
// internal/middleware/security.go
package middleware

func SecurityHeaders(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("X-XSS-Protection", "1; mode=block")
        w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
        w.Header().Set("Content-Security-Policy", "default-src 'self'")
        w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
        
        next.ServeHTTP(w, r)
    })
}
```

### Environment Configuration
```bash
# .env.example
# Application
SERVICE_NAME=api
SERVICE_VERSION=1.0.0
ENVIRONMENT=production
LOG_LEVEL=info
LOG_FORMAT=json

# Server
HTTP_PORT=8080
HTTP_READ_TIMEOUT=15s
HTTP_WRITE_TIMEOUT=15s
HTTP_IDLE_TIMEOUT=60s

# Database
DB_HOST=localhost
DB_PORT=5432
DB_USER=apiuser
DB_PASSWORD=changeme
DB_NAME=apidb
DB_SSL_MODE=require
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=5
DB_CONN_MAX_LIFETIME=5m

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0

# JWT
JWT_SECRET=your-secret-key
JWT_EXPIRATION=24h

# Monitoring
METRICS_ENABLED=true
TRACING_ENABLED=true
JAEGER_ENDPOINT=http://jaeger:14268/api/traces
```

## Deployment Checklist

1. **Security**
   - [ ] Use non-root user in container
   - [ ] Scan images for vulnerabilities
   - [ ] Use secrets management
   - [ ] Enable TLS/SSL
   - [ ] Implement rate limiting

2. **Reliability**
   - [ ] Configure health checks
   - [ ] Implement graceful shutdown
   - [ ] Set up monitoring and alerts
   - [ ] Configure autoscaling
   - [ ] Implement circuit breakers

3. **Performance**
   - [ ] Enable caching
   - [ ] Configure connection pools
   - [ ] Optimize container size
   - [ ] Set resource limits
   - [ ] Enable compression

4. **Observability**
   - [ ] Structured logging
   - [ ] Distributed tracing
   - [ ] Metrics collection
   - [ ] Error tracking
   - [ ] Performance monitoring

5. **Operations**
   - [ ] Automated CI/CD
   - [ ] Database migrations
   - [ ] Backup strategies
   - [ ] Disaster recovery plan
   - [ ] Documentation

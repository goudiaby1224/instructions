# Performance Optimization for Go APIs

## Profiling and Benchmarking

### CPU Profiling
```go
// cmd/api/main.go
import (
    "net/http"
    _ "net/http/pprof"
    "runtime"
)

func main() {
    // Enable profiling endpoint
    go func() {
        runtime.SetBlockProfileRate(1)
        runtime.SetMutexProfileFraction(1)
        log.Println(http.ListenAndServe("localhost:6060", nil))
    }()
    
    // Your application code
}

// Profile using:
// go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30
// go tool pprof http://localhost:6060/debug/pprof/heap
// go tool pprof http://localhost:6060/debug/pprof/goroutine
```

### Custom Metrics
```go
// internal/metrics/metrics.go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    requestDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "http_request_duration_seconds",
            Help: "Duration of HTTP requests.",
            Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
        },
        []string{"method", "path", "status"},
    )
    
    requestsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "http_requests_total",
            Help: "Total number of HTTP requests.",
        },
        []string{"method", "path", "status"},
    )
    
    activeConnections = promauto.NewGauge(
        prometheus.GaugeOpts{
            Name: "http_active_connections",
            Help: "Number of active HTTP connections.",
        },
    )
)

func MetricsMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
            requestDuration.WithLabelValues(r.Method, r.URL.Path, "").Observe(v)
        }))
        defer timer.ObserveDuration()
        
        wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
        
        activeConnections.Inc()
        defer activeConnections.Dec()
        
        next.ServeHTTP(wrapped, r)
        
        requestsTotal.WithLabelValues(r.Method, r.URL.Path, 
            http.StatusText(wrapped.statusCode)).Inc()
    })
}
```

## Memory Optimization

### Object Pooling
```go
// internal/pool/buffer_pool.go
package pool

import (
    "bytes"
    "sync"
)

var bufferPool = sync.Pool{
    New: func() interface{} {
        return new(bytes.Buffer)
    },
}

func GetBuffer() *bytes.Buffer {
    return bufferPool.Get().(*bytes.Buffer)
}

func PutBuffer(buf *bytes.Buffer) {
    buf.Reset()
    bufferPool.Put(buf)
}

// Usage example
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    buf := pool.GetBuffer()
    defer pool.PutBuffer(buf)
    
    // Use buffer for response building
    json.NewEncoder(buf).Encode(response)
    w.Write(buf.Bytes())
}
```

### Struct Optimization
```go
// Bad: 40 bytes due to padding
type User struct {
    Active bool    // 1 byte
    ID     int64   // 8 bytes
    Age    int32   // 4 bytes
    Name   string  // 16 bytes
}

// Good: 32 bytes with better alignment
type User struct {
    ID     int64   // 8 bytes
    Name   string  // 16 bytes
    Age    int32   // 4 bytes
    Active bool    // 1 byte + 3 padding
}

// Use fieldalignment tool to check:
// go install golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment@latest
// fieldalignment -fix ./...
```

## Database Query Optimization

### Connection Pool Tuning
```go
// internal/database/config.go
func OptimizeConnectionPool(db *sql.DB) {
    // Set maximum number of open connections
    db.SetMaxOpenConns(25)
    
    // Set maximum number of idle connections
    db.SetMaxIdleConns(5)
    
    // Set maximum lifetime of a connection
    db.SetConnMaxLifetime(5 * time.Minute)
    
    // Set maximum idle time
    db.SetConnMaxIdleTime(1 * time.Minute)
}
```

### Query Optimization
```go
// internal/repository/optimized_queries.go
package repository

// Use prepared statements
type OptimizedUserRepo struct {
    db         *sql.DB
    getByID    *sql.Stmt
    listUsers  *sql.Stmt
    updateUser *sql.Stmt
}

func NewOptimizedUserRepo(db *sql.DB) (*OptimizedUserRepo, error) {
    repo := &OptimizedUserRepo{db: db}
    
    var err error
    repo.getByID, err = db.Prepare(`
        SELECT id, email, name, created_at 
        FROM users 
        WHERE id = $1 AND deleted_at IS NULL
    `)
    if err != nil {
        return nil, err
    }
    
    repo.listUsers, err = db.Prepare(`
        SELECT id, email, name, created_at 
        FROM users 
        WHERE deleted_at IS NULL 
        ORDER BY created_at DESC 
        LIMIT $1 OFFSET $2
    `)
    if err != nil {
        return nil, err
    }
    
    return repo, nil
}

// Batch operations
func (r *OptimizedUserRepo) CreateBatch(ctx context.Context, users []*User) error {
    tx, err := r.db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()
    
    stmt, err := tx.Prepare(`
        INSERT INTO users (id, email, name, created_at) 
        VALUES ($1, $2, $3, $4)
    `)
    if err != nil {
        return err
    }
    defer stmt.Close()
    
    for _, user := range users {
        _, err := stmt.ExecContext(ctx, user.ID, user.Email, user.Name, user.CreatedAt)
        if err != nil {
            return err
        }
    }
    
    return tx.Commit()
}

// Use indexes effectively
/*
CREATE INDEX idx_users_email ON users(email) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_created_at ON users(created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_orders_user_id_created_at ON orders(user_id, created_at DESC);
*/
```

## Caching Strategies

### In-Memory Caching
```go
// internal/cache/memory.go
package cache

import (
    "sync"
    "time"
)

type MemoryCache struct {
    data    map[string]cacheItem
    mu      sync.RWMutex
    janitor *janitor
}

type cacheItem struct {
    value      interface{}
    expiration int64
}

func NewMemoryCache(cleanupInterval time.Duration) *MemoryCache {
    c := &MemoryCache{
        data: make(map[string]cacheItem),
    }
    
    c.janitor = &janitor{
        interval: cleanupInterval,
        stop:     make(chan bool),
    }
    
    go c.janitor.run(c)
    
    return c
}

func (c *MemoryCache) Set(key string, value interface{}, ttl time.Duration) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    expiration := time.Now().Add(ttl).UnixNano()
    c.data[key] = cacheItem{
        value:      value,
        expiration: expiration,
    }
}

func (c *MemoryCache) Get(key string) (interface{}, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    
    item, found := c.data[key]
    if !found {
        return nil, false
    }
    
    if item.expiration > 0 && time.Now().UnixNano() > item.expiration {
        return nil, false
    }
    
    return item.value, true
}
```

### Redis Caching
```go
// internal/cache/redis.go
package cache

import (
    "context"
    "encoding/json"
    "time"
    
    "github.com/go-redis/redis/v8"
)

type RedisCache struct {
    client *redis.Client
    prefix string
}

func (c *RedisCache) GetUser(ctx context.Context, id string) (*User, error) {
    key := c.prefix + ":user:" + id
    
    data, err := c.client.Get(ctx, key).Bytes()
    if err == redis.Nil {
        return nil, nil // Cache miss
    }
    if err != nil {
        return nil, err
    }
    
    var user User
    if err := json.Unmarshal(data, &user); err != nil {
        return nil, err
    }
    
    return &user, nil
}

func (c *RedisCache) SetUser(ctx context.Context, user *User, ttl time.Duration) error {
    key := c.prefix + ":user:" + user.ID
    
    data, err := json.Marshal(user)
    if err != nil {
        return err
    }
    
    return c.client.Set(ctx, key, data, ttl).Err()
}

// Cache-aside pattern
func (s *UserService) GetByID(ctx context.Context, id string) (*User, error) {
    // Try cache first
    user, err := s.cache.GetUser(ctx, id)
    if err != nil {
        log.Warn("cache error", "error", err)
    }
    if user != nil {
        return user, nil
    }
    
    // Cache miss, get from database
    user, err = s.repo.GetByID(ctx, id)
    if err != nil {
        return nil, err
    }
    
    // Update cache asynchronously
    go func() {
        if err := s.cache.SetUser(context.Background(), user, 5*time.Minute); err != nil {
            log.Warn("failed to update cache", "error", err)
        }
    }()
    
    return user, nil
}
```

## Concurrency Optimization

### Worker Pool Pattern
```go
// internal/workers/pool.go
package workers

type WorkerPool struct {
    workers   int
    jobQueue  chan Job
    workerWG  sync.WaitGroup
    quitChan  chan struct{}
}

func NewWorkerPool(workers int, queueSize int) *WorkerPool {
    return &WorkerPool{
        workers:  workers,
        jobQueue: make(chan Job, queueSize),
        quitChan: make(chan struct{}),
    }
}

func (p *WorkerPool) Start() {
    for i := 0; i < p.workers; i++ {
        p.workerWG.Add(1)
        go p.worker(i)
    }
}

func (p *WorkerPool) worker(id int) {
    defer p.workerWG.Done()
    
    for {
        select {
        case job := <-p.jobQueue:
            if err := job.Execute(); err != nil {
                log.Error("job failed", "worker", id, "error", err)
            }
        case <-p.quitChan:
            return
        }
    }
}

func (p *WorkerPool) Submit(job Job) error {
    select {
    case p.jobQueue <- job:
        return nil
    default:
        return errors.New("job queue full")
    }
}
```

### Optimized HTTP Client
```go
// internal/httpclient/client.go
package httpclient

import (
    "net"
    "net/http"
    "time"
)

func NewOptimizedClient() *http.Client {
    transport := &http.Transport{
        Proxy: http.ProxyFromEnvironment,
        DialContext: (&net.Dialer{
            Timeout:   30 * time.Second,
            KeepAlive: 30 * time.Second,
        }).DialContext,
        ForceAttemptHTTP2:     true,
        MaxIdleConns:          100,
        MaxIdleConnsPerHost:   10,
        IdleConnTimeout:       90 * time.Second,
        TLSHandshakeTimeout:   10 * time.Second,
        ExpectContinueTimeout: 1 * time.Second,
        DisableCompression:    false,
    }
    
    return &http.Client{
        Transport: transport,
        Timeout:   30 * time.Second,
    }
}
```

## Response Optimization

### Compression Middleware
```go
// internal/middleware/compression.go
package middleware

import (
    "compress/gzip"
    "io"
    "net/http"
    "strings"
)

func Gzip(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
            next.ServeHTTP(w, r)
            return
        }
        
        w.Header().Set("Content-Encoding", "gzip")
        gz := gzip.NewWriter(w)
        defer gz.Close()
        
        gzw := gzipResponseWriter{Writer: gz, ResponseWriter: w}
        next.ServeHTTP(gzw, r)
    })
}

type gzipResponseWriter struct {
    io.Writer
    http.ResponseWriter
}

func (w gzipResponseWriter) Write(b []byte) (int, error) {
    return w.Writer.Write(b)
}
```

### Pagination and Cursor-Based Pagination
```go
// internal/models/pagination.go
package models

type CursorPagination struct {
    Cursor string `json:"cursor,omitempty"`
    Limit  int    `json:"limit"`
}

type PagedResponse struct {
    Data       interface{} `json:"data"`
    NextCursor string      `json:"next_cursor,omitempty"`
    HasMore    bool        `json:"has_more"`
}

// Efficient cursor-based query
func (r *UserRepo) ListWithCursor(ctx context.Context, cursor string, limit int) ([]*User, string, error) {
    query := `
        SELECT id, email, name, created_at
        FROM users
        WHERE deleted_at IS NULL
        AND created_at < $1
        ORDER BY created_at DESC
        LIMIT $2
    `
    
    rows, err := r.db.QueryContext(ctx, query, cursor, limit+1)
    if err != nil {
        return nil, "", err
    }
    defer rows.Close()
    
    users := make([]*User, 0, limit)
    var nextCursor string
    
    for rows.Next() {
        var user User
        if err := rows.Scan(&user.ID, &user.Email, &user.Name, &user.CreatedAt); err != nil {
            return nil, "", err
        }
        
        if len(users) < limit {
            users = append(users, &user)
        } else {
            nextCursor = user.CreatedAt.Format(time.RFC3339Nano)
        }
    }
    
    return users, nextCursor, nil
}
```

## Performance Best Practices

1. **Profile before optimizing** - Use pprof to identify bottlenecks
2. **Use connection pooling** - Configure database and HTTP client pools
3. **Implement caching** - Cache frequently accessed data
4. **Optimize allocations** - Use sync.Pool for frequently allocated objects
5. **Batch operations** - Reduce database round trips
6. **Use prepared statements** - Improve query performance
7. **Enable compression** - Reduce network bandwidth
8. **Implement pagination** - Don't load entire datasets
9. **Monitor metrics** - Track performance indicators
10. **Load test regularly** - Identify performance regressions early

## Load Testing Example

```go
// tests/load/load_test.go
package load

import (
    "testing"
    "github.com/tsenart/vegeta/v12/lib"
)

func TestAPILoad(t *testing.T) {
    rate := vegeta.Rate{Freq: 100, Per: time.Second}
    duration := 30 * time.Second
    
    targeter := vegeta.NewStaticTargeter(vegeta.Target{
        Method: "GET",
        URL:    "http://localhost:8080/api/v1/users",
        Header: http.Header{
            "Authorization": []string{"Bearer " + token},
        },
    })
    
    attacker := vegeta.NewAttacker()
    
    var metrics vegeta.Metrics
    for res := range attacker.Attack(targeter, rate, duration, "Load Test") {
        metrics.Add(res)
    }
    metrics.Close()
    
    if metrics.Success < 0.95 {
        t.Errorf("Success rate too low: %.2f", metrics.Success)
    }
    
    if metrics.Latencies.P99 > 100*time.Millisecond {
        t.Errorf("P99 latency too high: %s", metrics.Latencies.P99)
    }
}
```

# HTTP Server Setup Best Practices

## Basic HTTP Server

```go
package main

import (
    "context"
    "log"
    "net/http"
    "os"
    "os/signal"
    "time"
)

func main() {
    mux := http.NewServeMux()
    
    srv := &http.Server{
        Addr:         ":8080",
        Handler:      mux,
        ReadTimeout:  15 * time.Second,
        WriteTimeout: 15 * time.Second,
        IdleTimeout:  60 * time.Second,
    }
    
    // Graceful shutdown
    go func() {
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("listen: %s\n", err)
        }
    }()
    
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, os.Interrupt)
    <-quit
    
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    if err := srv.Shutdown(ctx); err != nil {
        log.Fatal("Server forced to shutdown:", err)
    }
}
```

## Using Popular Frameworks

### Gin Framework
```go
import "github.com/gin-gonic/gin"

func setupGinServer() *gin.Engine {
    r := gin.New()
    
    // Middleware
    r.Use(gin.Logger())
    r.Use(gin.Recovery())
    
    // Routes
    api := r.Group("/api/v1")
    {
        api.GET("/users", getUsers)
        api.POST("/users", createUser)
    }
    
    return r
}
```

### Echo Framework
```go
import "github.com/labstack/echo/v4"

func setupEchoServer() *echo.Echo {
    e := echo.New()
    
    // Middleware
    e.Use(middleware.Logger())
    e.Use(middleware.Recover())
    
    // Routes
    e.GET("/api/v1/users", getUsers)
    e.POST("/api/v1/users", createUser)
    
    return e
}
```

### Fiber Framework
```go
import "github.com/gofiber/fiber/v2"

func setupFiberServer() *fiber.App {
    app := fiber.New(fiber.Config{
        ReadTimeout:  15 * time.Second,
        WriteTimeout: 15 * time.Second,
    })
    
    // Middleware
    app.Use(logger.New())
    app.Use(recover.New())
    
    // Routes
    api := app.Group("/api/v1")
    api.Get("/users", getUsers)
    api.Post("/users", createUser)
    
    return app
}
```

## Server Configuration Best Practices

1. **Always set timeouts**
2. **Implement graceful shutdown**
3. **Use structured logging**
4. **Add request ID middleware**
5. **Implement rate limiting**
6. **Add CORS when needed**
7. **Use TLS in production**

## Example Production-Ready Server

```go
type Server struct {
    httpServer *http.Server
    router     *mux.Router
    config     *Config
}

func NewServer(cfg *Config) *Server {
    router := mux.NewRouter()
    
    srv := &Server{
        router: router,
        config: cfg,
        httpServer: &http.Server{
            Addr:         cfg.Address,
            Handler:      router,
            ReadTimeout:  cfg.ReadTimeout,
            WriteTimeout: cfg.WriteTimeout,
            IdleTimeout:  cfg.IdleTimeout,
        },
    }
    
    srv.setupRoutes()
    srv.setupMiddleware()
    
    return srv
}

func (s *Server) Start() error {
    return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
    return s.httpServer.Shutdown(ctx)
}
```

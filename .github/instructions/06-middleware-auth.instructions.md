# Middleware & Authentication Best Practices

## Middleware Chain Pattern

```go
// internal/middleware/middleware.go
package middleware

type Middleware func(http.Handler) http.Handler

func Chain(middlewares ...Middleware) Middleware {
    return func(next http.Handler) http.Handler {
        for i := len(middlewares) - 1; i >= 0; i-- {
            next = middlewares[i](next)
        }
        return next
    }
}
```

## Request ID Middleware

```go
// internal/middleware/request_id.go
package middleware

import (
    "context"
    "github.com/google/uuid"
)

type contextKey string

const RequestIDKey contextKey = "requestID"

func RequestID(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        requestID := r.Header.Get("X-Request-ID")
        if requestID == "" {
            requestID = uuid.New().String()
        }
        
        ctx := context.WithValue(r.Context(), RequestIDKey, requestID)
        w.Header().Set("X-Request-ID", requestID)
        
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

func GetRequestID(ctx context.Context) string {
    if id, ok := ctx.Value(RequestIDKey).(string); ok {
        return id
    }
    return ""
}
```

## Logging Middleware

```go
// internal/middleware/logging.go
package middleware

import (
    "time"
    "github.com/go-chi/chi/v5/middleware"
)

func Logger(logger *slog.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()
            
            ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
            
            defer func() {
                logger.Info("HTTP Request",
                    "method", r.Method,
                    "path", r.URL.Path,
                    "status", ww.Status(),
                    "bytes", ww.BytesWritten(),
                    "duration", time.Since(start),
                    "request_id", GetRequestID(r.Context()),
                )
            }()
            
            next.ServeHTTP(ww, r)
        })
    }
}
```

## JWT Authentication

```go
// internal/auth/jwt.go
package auth

import (
    "github.com/golang-jwt/jwt/v4"
)

type Claims struct {
    UserID string `json:"user_id"`
    Email  string `json:"email"`
    Roles  []string `json:"roles"`
    jwt.RegisteredClaims
}

type JWTManager struct {
    secretKey     string
    tokenDuration time.Duration
}

func NewJWTManager(secretKey string, tokenDuration time.Duration) *JWTManager {
    return &JWTManager{
        secretKey:     secretKey,
        tokenDuration: tokenDuration,
    }
}

func (m *JWTManager) Generate(userID, email string, roles []string) (string, error) {
    claims := &Claims{
        UserID: userID,
        Email:  email,
        Roles:  roles,
        RegisteredClaims: jwt.RegisteredClaims{
            ExpiresAt: jwt.NewNumericDate(time.Now().Add(m.tokenDuration)),
            IssuedAt:  jwt.NewNumericDate(time.Now()),
            NotBefore: jwt.NewNumericDate(time.Now()),
        },
    }
    
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString([]byte(m.secretKey))
}

func (m *JWTManager) Verify(tokenString string) (*Claims, error) {
    token, err := jwt.ParseWithClaims(
        tokenString,
        &Claims{},
        func(token *jwt.Token) (interface{}, error) {
            if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
                return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
            }
            return []byte(m.secretKey), nil
        },
    )
    
    if err != nil {
        return nil, err
    }
    
    claims, ok := token.Claims.(*Claims)
    if !ok || !token.Valid {
        return nil, fmt.Errorf("invalid token")
    }
    
    return claims, nil
}
```

## Auth Middleware

```go
// internal/middleware/auth.go
package middleware

const UserContextKey contextKey = "user"

func Authenticate(jwtManager *auth.JWTManager) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            authHeader := r.Header.Get("Authorization")
            if authHeader == "" {
                http.Error(w, "Authorization header required", http.StatusUnauthorized)
                return
            }
            
            tokenString := strings.TrimPrefix(authHeader, "Bearer ")
            if tokenString == authHeader {
                http.Error(w, "Invalid authorization header format", http.StatusUnauthorized)
                return
            }
            
            claims, err := jwtManager.Verify(tokenString)
            if err != nil {
                http.Error(w, "Invalid token", http.StatusUnauthorized)
                return
            }
            
            ctx := context.WithValue(r.Context(), UserContextKey, claims)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

func RequireRoles(roles ...string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            claims, ok := r.Context().Value(UserContextKey).(*auth.Claims)
            if !ok {
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
            }
            
            for _, requiredRole := range roles {
                for _, userRole := range claims.Roles {
                    if userRole == requiredRole {
                        next.ServeHTTP(w, r)
                        return
                    }
                }
            }
            
            http.Error(w, "Forbidden", http.StatusForbidden)
        })
    }
}
```

## Rate Limiting Middleware

```go
// internal/middleware/rate_limit.go
package middleware

import (
    "golang.org/x/time/rate"
    "sync"
)

type IPRateLimiter struct {
    ips map[string]*rate.Limiter
    mu  *sync.RWMutex
    r   rate.Limit
    b   int
}

func NewIPRateLimiter(r rate.Limit, b int) *IPRateLimiter {
    return &IPRateLimiter{
        ips: make(map[string]*rate.Limiter),
        mu:  &sync.RWMutex{},
        r:   r,
        b:   b,
    }
}

func (i *IPRateLimiter) GetLimiter(ip string) *rate.Limiter {
    i.mu.Lock()
    defer i.mu.Unlock()
    
    limiter, exists := i.ips[ip]
    if !exists {
        limiter = rate.NewLimiter(i.r, i.b)
        i.ips[ip] = limiter
    }
    
    return limiter
}

func RateLimit(limiter *IPRateLimiter) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            ip := getIP(r)
            l := limiter.GetLimiter(ip)
            
            if !l.Allow() {
                http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
                return
            }
            
            next.ServeHTTP(w, r)
        })
    }
}
```

## CORS Middleware

```go
// internal/middleware/cors.go
package middleware

func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            origin := r.Header.Get("Origin")
            
            for _, allowed := range allowedOrigins {
                if origin == allowed || allowed == "*" {
                    w.Header().Set("Access-Control-Allow-Origin", origin)
                    break
                }
            }
            
            w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
            w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
            w.Header().Set("Access-Control-Max-Age", "86400")
            
            if r.Method == "OPTIONS" {
                w.WriteHeader(http.StatusOK)
                return
            }
            
            next.ServeHTTP(w, r)
        })
    }
}
```

## API Key Authentication

```go
// internal/auth/apikey.go
package auth

type APIKeyAuth struct {
    store APIKeyStore
}

func (a *APIKeyAuth) Authenticate(apiKey string) (*APIKeyInfo, error) {
    info, err := a.store.GetAPIKeyInfo(apiKey)
    if err != nil {
        return nil, err
    }
    
    if info.ExpiresAt.Before(time.Now()) {
        return nil, fmt.Errorf("API key expired")
    }
    
    if !info.Active {
        return nil, fmt.Errorf("API key inactive")
    }
    
    return info, nil
}

func APIKeyMiddleware(auth *APIKeyAuth) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            apiKey := r.Header.Get("X-API-Key")
            if apiKey == "" {
                http.Error(w, "API key required", http.StatusUnauthorized)
                return
            }
            
            info, err := auth.Authenticate(apiKey)
            if err != nil {
                http.Error(w, "Invalid API key", http.StatusUnauthorized)
                return
            }
            
            ctx := context.WithValue(r.Context(), "api_key_info", info)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

## Usage Example

```go
// cmd/api/main.go
func setupRoutes(r *chi.Mux) {
    // Global middleware
    r.Use(middleware.RequestID)
    r.Use(middleware.Logger(logger))
    r.Use(middleware.Recoverer)
    r.Use(middleware.RateLimit(rateLimiter))
    r.Use(middleware.CORS([]string{"*"}))
    
    // Public routes
    r.Group(func(r chi.Router) {
        r.Post("/api/v1/auth/login", handlers.Login)
        r.Post("/api/v1/auth/register", handlers.Register)
    })
    
    // Protected routes
    r.Group(func(r chi.Router) {
        r.Use(middleware.Authenticate(jwtManager))
        
        r.Get("/api/v1/users/me", handlers.GetCurrentUser)
        r.Put("/api/v1/users/me", handlers.UpdateCurrentUser)
        
        // Admin only routes
        r.Group(func(r chi.Router) {
            r.Use(middleware.RequireRoles("admin"))
            
            r.Get("/api/v1/users", handlers.ListUsers)
            r.Delete("/api/v1/users/{id}", handlers.DeleteUser)
        })
    })
}
```

# Go API Project Structure

## Standard Project Layout

```
myapi/
├── cmd/
│   └── api/
│       └── main.go
├── internal/
│   ├── config/
│   ├── handlers/
│   ├── middleware/
│   ├── models/
│   ├── repository/
│   └── services/
├── pkg/
│   ├── errors/
│   └── utils/
├── api/
│   └── openapi.yaml
├── scripts/
├── configs/
├── deployments/
├── docs/
├── go.mod
├── go.sum
├── Makefile
├── README.md
└── .gitignore
```

## Hexagonal Architecture Layout

```
myapi/
├── cmd/
│   └── api/
│       └── main.go
├── internal/
│   ├── core/                 # Business logic (hexagon center)
│   │   ├── domain/          # Domain models and business rules
│   │   │   ├── user.go
│   │   │   └── order.go
│   │   ├── ports/           # Interfaces (contracts)
│   │   │   ├── incoming/    # Input ports (use cases)
│   │   │   │   ├── user_service.go
│   │   │   │   └── order_service.go
│   │   │   └── outgoing/    # Output ports (repositories)
│   │   │       ├── user_repository.go
│   │   │       ├── order_repository.go
│   │   │       └── notification_service.go
│   │   └── services/        # Use case implementations
│   │       ├── user_service.go
│   │       └── order_service.go
│   └── adapters/            # External adapters
│       ├── incoming/        # Driving adapters (API, CLI, etc.)
│       │   ├── http/        # HTTP REST handlers
│       │   │   ├── server.go
│       │   │   ├── user_handler.go
│       │   │   └── order_handler.go
│       │   ├── grpc/        # gRPC handlers
│       │   │   └── server.go
│       │   └── cli/         # CLI commands
│       │       └── commands.go
│       └── outgoing/        # Driven adapters (DB, external services)
│           ├── database/    # Database implementations
│           │   ├── postgres/
│           │   │   ├── user_repository.go
│           │   │   └── order_repository.go
│           │   └── mongodb/
│           │       └── user_repository.go
│           ├── cache/       # Cache implementations
│           │   └── redis/
│           │       └── cache_service.go
│           └── external/    # External service clients
│               ├── payment/
│               │   └── stripe_client.go
│               └── email/
│                   └── sendgrid_client.go
├── pkg/                     # Shared utilities
│   ├── errors/
│   └── utils/
├── api/
│   └── openapi.yaml
├── configs/
├── deployments/
├── docs/
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

## Directory Explanations

### `/cmd`
- Main applications for this project
- Directory name should match executable name
- Keep main.go minimal

### `/internal`
- Private application code
- Not importable by other projects
- Contains business logic

### `/pkg`
- Library code that's safe to use by external applications
- Public APIs

### `/api`
- OpenAPI/Swagger specs, JSON schema files
- Protocol definition files

### Hexagonal Architecture Directories

#### `/internal/core`
- Contains all business logic
- Independent of external frameworks and technologies
- Should be testable without any external dependencies

##### `/internal/core/domain`
- Domain entities and value objects
- Business rules and invariants
- No dependencies on external packages

##### `/internal/core/ports`
- Interfaces that define contracts
- `incoming/`: Input ports (what the application can do)
- `outgoing/`: Output ports (what the application needs)

##### `/internal/core/services`
- Implementation of input ports
- Orchestrates business logic
- Uses output ports for external interactions

#### `/internal/adapters`
- Implements the ports defined in core
- Translates between external formats and domain models

##### `/internal/adapters/incoming`
- Driving adapters that trigger use cases
- Examples: HTTP handlers, gRPC services, CLI commands
- Depends on input ports

##### `/internal/adapters/outgoing`
- Driven adapters that are called by the core
- Examples: Database repositories, cache services, external APIs
- Implements output ports

## Best Practices

1. **Use `internal/` for private code**
```go
// internal/handlers/user.go
package handlers

type UserHandler struct {
    service services.UserService
}
```

2. **Keep `main.go` simple**
```go
// cmd/api/main.go
package main

import (
    "log"
    "myapi/internal/config"
    "myapi/internal/server"
)

func main() {
    cfg := config.Load()
    srv := server.New(cfg)
    log.Fatal(srv.Start())
}
```

3. **Use interfaces for dependencies**
```go
// internal/services/interfaces.go
package services

type UserRepository interface {
    GetByID(id string) (*User, error)
    Create(user *User) error
}
```

4. **Separate concerns**
- Handlers: HTTP request/response handling
- Services: Business logic
- Repository: Data access
- Models: Data structures

5. **Implement Dependency Inversion**
```go
// internal/core/ports/outgoing/user_repository.go
package outgoing

type UserRepository interface {
    Save(ctx context.Context, user *domain.User) error
    FindByID(ctx context.Context, id string) (*domain.User, error)
}

// internal/adapters/outgoing/database/postgres/user_repository.go
package postgres

type PostgresUserRepository struct {
    db *sql.DB
}

func (r *PostgresUserRepository) Save(ctx context.Context, user *domain.User) error {
    // Implementation
}
```

6. **Wire dependencies in main.go**
```go
// cmd/api/main.go
package main

import (
    "myapi/internal/adapters/incoming/http"
    "myapi/internal/adapters/outgoing/database/postgres"
    "myapi/internal/core/services"
)

func main() {
    // Initialize outgoing adapters
    userRepo := postgres.NewUserRepository(db)
    
    // Initialize services with outgoing adapters
    userService := services.NewUserService(userRepo)
    
    // Initialize incoming adapters with services
    httpServer := http.NewServer(userService)
    
    // Start server
    httpServer.Start()
}
```

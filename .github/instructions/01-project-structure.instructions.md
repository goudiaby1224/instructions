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
│       │   ├── http/        # Synchronous HTTP REST handlers
│       │   │   ├── server.go
│       │   │   ├── user_handler.go
│       │   │   └── order_handler.go
│       │   ├── grpc/        # Synchronous gRPC handlers
│       │   │   └── server.go
│       │   ├── messaging/   # Asynchronous message handlers
│       │   │   ├── kafka/
│       │   │   │   ├── consumer.go
│       │   │   │   ├── user_events_handler.go
│       │   │   │   └── order_events_handler.go
│       │   │   ├── rabbitmq/
│       │   │   │   ├── consumer.go
│       │   │   │   └── event_handler.go
│       │   │   ├── nats/
│       │   │   │   ├── subscriber.go
│       │   │   │   └── event_handler.go
│       │   │   └── redis/
│       │   │       ├── subscriber.go
│       │   │       └── pub_sub_handler.go
│       │   ├── websocket/   # Real-time WebSocket handlers
│       │   │   ├── hub.go
│       │   │   └── client.go
│       │   ├── cron/        # Scheduled job handlers
│       │   │   ├── scheduler.go
│       │   │   └── job_handlers.go
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
│           ├── messaging/   # Message publishing
│           │   ├── kafka/
│           │   │   └── producer.go
│           │   ├── rabbitmq/
│           │   │   └── publisher.go
│           │   └── nats/
│           │       └── publisher.go
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
- **Synchronous adapters**: HTTP handlers, gRPC services, CLI commands
- **Asynchronous adapters**: Kafka consumers, RabbitMQ consumers, NATS subscribers
- **Real-time adapters**: WebSocket handlers, Server-Sent Events
- **Scheduled adapters**: Cron jobs, scheduled tasks
- All depend on input ports

###### `/internal/adapters/incoming/messaging`
- Asynchronous message consumers and event handlers
- Each messaging system has its own subdirectory
- Handles message deserialization and routing to appropriate services
- Implements error handling, retries, and dead letter queues

###### `/internal/adapters/incoming/websocket`
- Real-time communication handlers
- Manages WebSocket connections and broadcasts
- Handles connection lifecycle and message routing

###### `/internal/adapters/incoming/cron`
- Scheduled job handlers
- Background task processing
- Periodic maintenance operations

##### `/internal/adapters/outgoing`
- Driven adapters that are called by the core
- Examples: Database repositories, cache services, external APIs, message publishers
- Implements output ports

## Asynchronous Incoming Adapter Examples

### Kafka Consumer Example
```go
// internal/adapters/incoming/messaging/kafka/user_events_handler.go
package kafka

import (
    "context"
    "encoding/json"
    "myapi/internal/core/ports/incoming"
)

type UserEventsHandler struct {
    userService incoming.UserService
    logger      Logger
}

func (h *UserEventsHandler) HandleUserCreated(ctx context.Context, message []byte) error {
    var event UserCreatedEvent
    if err := json.Unmarshal(message, &event); err != nil {
        return fmt.Errorf("failed to unmarshal user created event: %w", err)
    }
    
    // Convert event to domain model and call service
    user := event.ToDomainUser()
    return h.userService.ProcessUserCreated(ctx, user)
}

func (h *UserEventsHandler) HandleUserUpdated(ctx context.Context, message []byte) error {
    var event UserUpdatedEvent
    if err := json.Unmarshal(message, &event); err != nil {
        return fmt.Errorf("failed to unmarshal user updated event: %w", err)
    }
    
    user := event.ToDomainUser()
    return h.userService.ProcessUserUpdated(ctx, user)
}
```

### Message Consumer Setup
```go
// internal/adapters/incoming/messaging/kafka/consumer.go
package kafka

type Consumer struct {
    reader       *kafka.Reader
    handlers     map[string]MessageHandler
    logger       Logger
    errorHandler ErrorHandler
}

func (c *Consumer) Subscribe(topic string, handler MessageHandler) {
    c.handlers[topic] = handler
}

func (c *Consumer) Start(ctx context.Context) error {
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
            message, err := c.reader.ReadMessage(ctx)
            if err != nil {
                c.logger.Error("failed to read message", "error", err)
                continue
            }
            
            handler, exists := c.handlers[message.Topic]
            if !exists {
                c.logger.Warn("no handler for topic", "topic", message.Topic)
                continue
            }
            
            if err := handler.Handle(ctx, message.Value); err != nil {
                c.errorHandler.Handle(ctx, message, err)
            }
        }
    }
}
```

### Dependency Injection for Async Adapters
```go
// cmd/api/main.go
func main() {
    // Initialize outgoing adapters
    userRepo := postgres.NewUserRepository(db)
    
    // Initialize services
    userService := services.NewUserService(userRepo)
    
    // Initialize synchronous incoming adapters
    httpServer := http.NewServer(userService)
    
    // Initialize asynchronous incoming adapters
    kafkaConsumer := kafka.NewConsumer(kafkaConfig)
    userEventsHandler := kafka.NewUserEventsHandler(userService, logger)
    
    // Subscribe to topics
    kafkaConsumer.Subscribe("user.created", userEventsHandler.HandleUserCreated)
    kafkaConsumer.Subscribe("user.updated", userEventsHandler.HandleUserUpdated)
    
    // Start all adapters
    go httpServer.Start()
    go kafkaConsumer.Start(context.Background())
    
    // Graceful shutdown
    // ...
}
```

## Best Practices for Async Incoming Adapters

1. **Separate message handling from business logic**
2. **Use dependency injection for services**
3. **Implement proper error handling and retry mechanisms**
4. **Use structured logging with correlation IDs**
5. **Handle message deserialization errors gracefully**
6. **Implement idempotency for message processing**
7. **Use circuit breakers for external dependencies**
8. **Monitor message processing metrics**
9. **Implement dead letter queues for failed messages**
10. **Use context for cancellation and timeouts**

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

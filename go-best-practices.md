# Go Best Practices

## Hexagonal Architecture Best Practices

### 1. Port Definitions

Define clear interfaces for both incoming and outgoing ports:

```go
// internal/core/ports/incoming/user_service.go
type UserService interface {
    CreateUser(ctx context.Context, user *domain.User) error
    GetUser(ctx context.Context, id string) (*domain.User, error)
}

// internal/core/ports/outgoing/user_repository.go
type UserRepository interface {
    Save(ctx context.Context, user *domain.User) error
    FindByID(ctx context.Context, id string) (*domain.User, error)
}
```

### 2. Adapter Implementation

Keep adapters focused on translation between external and internal representations:

```go
// internal/adapters/incoming/http/user_handler.go
type UserHandler struct {
    userService ports.UserService
}

// internal/adapters/outgoing/database/postgres/user_repository.go
type PostgresUserRepository struct {
    db *sql.DB
}
```

### 3. Dependency Flow

- Dependencies always point inward (toward the core)
- Core never depends on adapters
- Adapters depend on core ports
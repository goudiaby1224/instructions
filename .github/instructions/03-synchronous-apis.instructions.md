# Synchronous API Development in Go

## RESTful API Design

### Standard HTTP Methods
- GET: Retrieve resources
- POST: Create new resources
- PUT: Update entire resources
- PATCH: Partial updates
- DELETE: Remove resources

## Hexagonal Architecture Handler Pattern

```go
// internal/adapters/incoming/http/user_handler.go
package http

import (
    "encoding/json"
    "net/http"
    "myapi/internal/core/ports/incoming"
)

type UserHandler struct {
    userService incoming.UserService
}

func NewUserHandler(userService incoming.UserService) *UserHandler {
    return &UserHandler{userService: userService}
}

// GET /api/v1/users
func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
    users, err := h.userService.GetAllUsers(r.Context())
    if err != nil {
        respondWithError(w, http.StatusInternalServerError, err.Error())
        return
    }
    
    // Convert domain models to DTOs
    response := make([]UserDTO, len(users))
    for i, user := range users {
        response[i] = toUserDTO(user)
    }
    
    respondWithJSON(w, http.StatusOK, response)
}

// POST /api/v1/users
func (h *UserHandler) Create(w http.ResponseWriter, r *http.Request) {
    var request CreateUserRequest
    if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
        respondWithError(w, http.StatusBadRequest, "Invalid request body")
        return
    }
    
    // Convert DTO to domain model
    user := toDomainUser(request)
    
    if err := h.userService.CreateUser(r.Context(), user); err != nil {
        respondWithError(w, http.StatusInternalServerError, err.Error())
        return
    }
    
    respondWithJSON(w, http.StatusCreated, toUserDTO(user))
}

// DTO definitions
type UserDTO struct {
    ID        string `json:"id"`
    Email     string `json:"email"`
    Name      string `json:"name"`
    CreatedAt string `json:"created_at"`
}

type CreateUserRequest struct {
    Email    string `json:"email" validate:"required,email"`
    Name     string `json:"name" validate:"required,min=2,max=100"`
    Password string `json:"password" validate:"required,min=8"`
}

// Conversion functions
func toUserDTO(user *domain.User) UserDTO {
    return UserDTO{
        ID:        user.ID,
        Email:     user.Email,
        Name:      user.Name,
        CreatedAt: user.CreatedAt.Format(time.RFC3339),
    }
}

func toDomainUser(req CreateUserRequest) *domain.User {
    return &domain.User{
        Email: req.Email,
        Name:  req.Name,
    }
}
```

## Port Definitions

```go
// internal/core/ports/incoming/user_service.go
package incoming

import (
    "context"
    "myapi/internal/core/domain"
)

type UserService interface {
    CreateUser(ctx context.Context, user *domain.User) error
    GetUser(ctx context.Context, id string) (*domain.User, error)
    GetAllUsers(ctx context.Context) ([]*domain.User, error)
    UpdateUser(ctx context.Context, user *domain.User) error
    DeleteUser(ctx context.Context, id string) error
}
```

## Service Implementation

```go
// internal/core/services/user_service.go
package services

import (
    "context"
    "myapi/internal/core/domain"
    "myapi/internal/core/ports/incoming"
    "myapi/internal/core/ports/outgoing"
)

type userService struct {
    userRepo outgoing.UserRepository
    logger   outgoing.Logger
}

func NewUserService(
    userRepo outgoing.UserRepository,
    logger outgoing.Logger,
) incoming.UserService {
    return &userService{
        userRepo: userRepo,
        logger:   logger,
    }
}

func (s *userService) CreateUser(ctx context.Context, user *domain.User) error {
    // Business logic validation
    if err := user.Validate(); err != nil {
        return err
    }
    
    // Check if email exists
    existing, err := s.userRepo.FindByEmail(ctx, user.Email)
    if err != nil && !errors.Is(err, domain.ErrNotFound) {
        return err
    }
    if existing != nil {
        return domain.ErrEmailAlreadyExists
    }
    
    // Generate ID and timestamps
    user.ID = uuid.New().String()
    user.CreatedAt = time.Now()
    user.UpdatedAt = time.Now()
    
    // Hash password
    if err := user.HashPassword(); err != nil {
        return err
    }
    
    // Save to repository
    if err := s.userRepo.Save(ctx, user); err != nil {
        s.logger.Error("failed to save user", "error", err)
        return err
    }
    
    return nil
}

func (s *userService) GetAllUsers(ctx context.Context) ([]*domain.User, error) {
    return s.userRepo.FindAll(ctx)
}
```

## Domain Model

```go
// internal/core/domain/user.go
package domain

import (
    "errors"
    "golang.org/x/crypto/bcrypt"
    "time"
)

type User struct {
    ID           string
    Email        string
    Name         string
    PasswordHash string
    CreatedAt    time.Time
    UpdatedAt    time.Time
}

func (u *User) Validate() error {
    if u.Email == "" {
        return errors.New("email is required")
    }
    if u.Name == "" {
        return errors.New("name is required")
    }
    // Add more validation rules
    return nil
}

func (u *User) HashPassword() error {
    // Business logic for password hashing
    hash, err := bcrypt.GenerateFromPassword([]byte(u.PasswordHash), bcrypt.DefaultCost)
    if err != nil {
        return err
    }
    u.PasswordHash = string(hash)
    return nil
}
```

## Request/Response Helpers

```go
// internal/handlers/helpers.go
package handlers

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
    response, _ := json.Marshal(payload)
    
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    w.Write(response)
}

func respondWithError(w http.ResponseWriter, code int, message string) {
    respondWithJSON(w, code, map[string]string{"error": message})
}
```

## Input Validation

```go
// internal/models/requests.go
package models

import "github.com/go-playground/validator/v10"

var validate = validator.New()

type CreateUserRequest struct {
    Name     string `json:"name" validate:"required,min=2,max=100"`
    Email    string `json:"email" validate:"required,email"`
    Password string `json:"password" validate:"required,min=8"`
}

func (r *CreateUserRequest) Validate() error {
    return validate.Struct(r)
}
```

## Service Layer Pattern

```go
// internal/services/user.go
package services

type UserService struct {
    repo UserRepository
    log  Logger
}

func (s *UserService) GetAll(ctx context.Context) ([]*User, error) {
    span, ctx := opentracing.StartSpanFromContext(ctx, "UserService.GetAll")
    defer span.Finish()
    
    users, err := s.repo.FindAll(ctx)
    if err != nil {
        s.log.Error("failed to get users", "error", err)
        return nil, fmt.Errorf("failed to get users: %w", err)
    }
    
    return users, nil
}

func (s *UserService) Create(ctx context.Context, input *CreateUserRequest) (*User, error) {
    // Validate business rules
    exists, err := s.repo.EmailExists(ctx, input.Email)
    if err != nil {
        return nil, fmt.Errorf("failed to check email: %w", err)
    }
    if exists {
        return nil, ErrEmailAlreadyExists
    }
    
    // Hash password
    hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
    if err != nil {
        return nil, fmt.Errorf("failed to hash password: %w", err)
    }
    
    user := &User{
        ID:       uuid.New().String(),
        Name:     input.Name,
        Email:    input.Email,
        Password: string(hashedPassword),
    }
    
    if err := s.repo.Create(ctx, user); err != nil {
        return nil, fmt.Errorf("failed to create user: %w", err)
    }
    
    return user, nil
}
```

## Best Practices

1. **Use context for request-scoped values**
2. **Implement proper pagination**
3. **Version your APIs**
4. **Use structured logging**
5. **Implement request/response validation**
6. **Handle errors consistently**
7. **Use dependency injection**
8. **Implement idempotency for POST/PUT**
9. **Keep adapters thin** - Business logic belongs in the core
10. **Use DTOs** - Don't expose domain models directly

# Synchronous API Development in Go

## RESTful API Design

### Standard HTTP Methods
- GET: Retrieve resources
- POST: Create new resources
- PUT: Update entire resources
- PATCH: Partial updates
- DELETE: Remove resources

## Handler Pattern

```go
// internal/handlers/user.go
package handlers

import (
    "encoding/json"
    "net/http"
)

type UserHandler struct {
    service UserService
}

func NewUserHandler(service UserService) *UserHandler {
    return &UserHandler{service: service}
}

// GET /api/v1/users
func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
    users, err := h.service.GetAll(r.Context())
    if err != nil {
        respondWithError(w, http.StatusInternalServerError, err.Error())
        return
    }
    
    respondWithJSON(w, http.StatusOK, users)
}

// GET /api/v1/users/{id}
func (h *UserHandler) Get(w http.ResponseWriter, r *http.Request) {
    id := mux.Vars(r)["id"]
    
    user, err := h.service.GetByID(r.Context(), id)
    if err != nil {
        if errors.Is(err, ErrNotFound) {
            respondWithError(w, http.StatusNotFound, "User not found")
            return
        }
        respondWithError(w, http.StatusInternalServerError, err.Error())
        return
    }
    
    respondWithJSON(w, http.StatusOK, user)
}

// POST /api/v1/users
func (h *UserHandler) Create(w http.ResponseWriter, r *http.Request) {
    var input CreateUserRequest
    if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
        respondWithError(w, http.StatusBadRequest, "Invalid request body")
        return
    }
    
    if err := input.Validate(); err != nil {
        respondWithError(w, http.StatusBadRequest, err.Error())
        return
    }
    
    user, err := h.service.Create(r.Context(), &input)
    if err != nil {
        respondWithError(w, http.StatusInternalServerError, err.Error())
        return
    }
    
    respondWithJSON(w, http.StatusCreated, user)
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

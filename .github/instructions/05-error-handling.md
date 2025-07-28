# Error Handling Best Practices

## Custom Error Types

```go
// pkg/errors/errors.go
package errors

import (
    "fmt"
    "net/http"
)

type AppError struct {
    Code    string `json:"code"`
    Message string `json:"message"`
    Status  int    `json:"-"`
    Cause   error  `json:"-"`
}

func (e *AppError) Error() string {
    if e.Cause != nil {
        return fmt.Sprintf("%s: %v", e.Message, e.Cause)
    }
    return e.Message
}

func (e *AppError) Unwrap() error {
    return e.Cause
}

// Predefined errors
var (
    ErrNotFound = &AppError{
        Code:    "NOT_FOUND",
        Message: "Resource not found",
        Status:  http.StatusNotFound,
    }
    
    ErrUnauthorized = &AppError{
        Code:    "UNAUTHORIZED",
        Message: "Unauthorized access",
        Status:  http.StatusUnauthorized,
    }
    
    ErrValidation = &AppError{
        Code:    "VALIDATION_ERROR",
        Message: "Validation failed",
        Status:  http.StatusBadRequest,
    }
    
    ErrInternal = &AppError{
        Code:    "INTERNAL_ERROR",
        Message: "Internal server error",
        Status:  http.StatusInternalServerError,
    }
)

// Constructor functions
func NewNotFound(message string) *AppError {
    return &AppError{
        Code:    "NOT_FOUND",
        Message: message,
        Status:  http.StatusNotFound,
    }
}

func NewValidation(message string) *AppError {
    return &AppError{
        Code:    "VALIDATION_ERROR",
        Message: message,
        Status:  http.StatusBadRequest,
    }
}

func Wrap(err error, message string) *AppError {
    return &AppError{
        Code:    "INTERNAL_ERROR",
        Message: message,
        Status:  http.StatusInternalServerError,
        Cause:   err,
    }
}
```

## Error Handling Middleware

```go
// internal/middleware/error.go
package middleware

func ErrorHandler(logger Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            defer func() {
                if err := recover(); err != nil {
                    logger.Error("panic recovered", 
                        "error", err,
                        "stack", debug.Stack(),
                        "path", r.URL.Path,
                    )
                    
                    respondWithError(w, http.StatusInternalServerError, "Internal server error")
                }
            }()
            
            next.ServeHTTP(w, r)
        })
    }
}

func respondWithError(w http.ResponseWriter, status int, message string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(map[string]interface{}{
        "error": map[string]string{
            "message": message,
        },
    })
}
```

## Service Layer Error Handling

```go
// internal/services/user.go
package services

func (s *UserService) GetByID(ctx context.Context, id string) (*User, error) {
    user, err := s.repo.FindByID(ctx, id)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, errors.NewNotFound("user not found")
        }
        return nil, errors.Wrap(err, "failed to get user")
    }
    
    return user, nil
}

func (s *UserService) Create(ctx context.Context, input *CreateUserInput) (*User, error) {
    // Validation
    if err := input.Validate(); err != nil {
        return nil, errors.NewValidation(err.Error())
    }
    
    // Check if email exists
    exists, err := s.repo.EmailExists(ctx, input.Email)
    if err != nil {
        return nil, errors.Wrap(err, "failed to check email existence")
    }
    if exists {
        return nil, errors.NewValidation("email already exists")
    }
    
    // Create user
    user := &User{
        ID:    uuid.New().String(),
        Email: input.Email,
        Name:  input.Name,
    }
    
    if err := s.repo.Create(ctx, user); err != nil {
        return nil, errors.Wrap(err, "failed to create user")
    }
    
    return user, nil
}
```

## Handler Error Response

```go
// internal/handlers/user.go
package handlers

func (h *UserHandler) Get(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    
    user, err := h.service.GetByID(r.Context(), id)
    if err != nil {
        h.handleError(w, err)
        return
    }
    
    respondWithJSON(w, http.StatusOK, user)
}

func (h *UserHandler) handleError(w http.ResponseWriter, err error) {
    var appErr *errors.AppError
    if errors.As(err, &appErr) {
        h.logger.Error("request failed", 
            "error", appErr.Error(),
            "code", appErr.Code,
        )
        
        respondWithJSON(w, appErr.Status, map[string]interface{}{
            "error": map[string]string{
                "code":    appErr.Code,
                "message": appErr.Message,
            },
        })
        return
    }
    
    // Unknown error
    h.logger.Error("unexpected error", "error", err)
    respondWithJSON(w, http.StatusInternalServerError, map[string]interface{}{
        "error": map[string]string{
            "code":    "INTERNAL_ERROR",
            "message": "An unexpected error occurred",
        },
    })
}
```

## Validation Errors

```go
// internal/models/validation.go
package models

type ValidationError struct {
    Field   string `json:"field"`
    Message string `json:"message"`
}

type ValidationErrors []ValidationError

func (v ValidationErrors) Error() string {
    return "validation failed"
}

func ValidateCreateUser(input *CreateUserInput) ValidationErrors {
    var errors ValidationErrors
    
    if input.Email == "" {
        errors = append(errors, ValidationError{
            Field:   "email",
            Message: "email is required",
        })
    } else if !isValidEmail(input.Email) {
        errors = append(errors, ValidationError{
            Field:   "email",
            Message: "invalid email format",
        })
    }
    
    if len(input.Password) < 8 {
        errors = append(errors, ValidationError{
            Field:   "password",
            Message: "password must be at least 8 characters",
        })
    }
    
    if len(errors) > 0 {
        return errors
    }
    
    return nil
}
```

## Error Logging Best Practices

```go
// internal/logger/logger.go
package logger

func LogError(ctx context.Context, err error, msg string, fields ...interface{}) {
    requestID := GetRequestID(ctx)
    userID := GetUserID(ctx)
    
    allFields := append([]interface{}{
        "error", err.Error(),
        "request_id", requestID,
        "user_id", userID,
    }, fields...)
    
    if appErr, ok := err.(*errors.AppError); ok {
        allFields = append(allFields, 
            "error_code", appErr.Code,
            "status", appErr.Status,
        )
        
        if appErr.Cause != nil {
            allFields = append(allFields, "cause", appErr.Cause.Error())
        }
    }
    
    logger.Error(msg, allFields...)
}
```

## Best Practices Summary

1. **Use custom error types** for domain-specific errors
2. **Wrap errors with context** using fmt.Errorf or errors.Wrap
3. **Check errors immediately** after function calls
4. **Use errors.Is and errors.As** for error comparison
5. **Log errors with context** (request ID, user ID, etc.)
6. **Return meaningful error messages** to clients
7. **Don't expose internal errors** to clients
8. **Use panic only for unrecoverable errors**
9. **Implement error metrics** for monitoring
10. **Test error scenarios** thoroughly

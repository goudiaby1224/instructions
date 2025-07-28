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

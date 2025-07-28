# Testing Strategies for Go APIs

## Unit Testing

### Basic Test Structure
```go
// internal/services/user_test.go
package services

import (
    "context"
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
)

func TestUserService_GetByID(t *testing.T) {
    tests := []struct {
        name    string
        userID  string
        setup   func(*mocks.UserRepository)
        want    *User
        wantErr bool
    }{
        {
            name:   "successful get",
            userID: "123",
            setup: func(m *mocks.UserRepository) {
                m.On("GetByID", mock.Anything, "123").Return(&User{
                    ID:    "123",
                    Email: "test@example.com",
                    Name:  "Test User",
                }, nil)
            },
            want: &User{
                ID:    "123",
                Email: "test@example.com",
                Name:  "Test User",
            },
            wantErr: false,
        },
        {
            name:   "user not found",
            userID: "456",
            setup: func(m *mocks.UserRepository) {
                m.On("GetByID", mock.Anything, "456").Return(nil, ErrNotFound)
            },
            want:    nil,
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            repo := new(mocks.UserRepository)
            tt.setup(repo)
            
            svc := NewUserService(repo)
            got, err := svc.GetByID(context.Background(), tt.userID)
            
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.Equal(t, tt.want, got)
            }
            
            repo.AssertExpectations(t)
        })
    }
}
```

### Mocking with Mockery

```go
// mocks/user_repository.go
// Generated using: mockery --name=UserRepository --dir=internal/repository --output=mocks
package mocks

import (
    "context"
    "github.com/stretchr/testify/mock"
)

type UserRepository struct {
    mock.Mock
}

func (m *UserRepository) GetByID(ctx context.Context, id string) (*User, error) {
    args := m.Called(ctx, id)
    
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    
    return args.Get(0).(*User), args.Error(1)
}
```

## Integration Testing

### Database Testing with TestContainers
```go
// internal/repository/user_test.go
package repository

import (
    "context"
    "database/sql"
    "testing"
    
    "github.com/golang-migrate/migrate/v4"
    "github.com/stretchr/testify/suite"
    "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/wait"
)

type UserRepositoryTestSuite struct {
    suite.Suite
    container testcontainers.Container
    db        *sql.DB
    repo      UserRepository
}

func (suite *UserRepositoryTestSuite) SetupSuite() {
    ctx := context.Background()
    
    // Start PostgreSQL container
    req := testcontainers.ContainerRequest{
        Image:        "postgres:15-alpine",
        ExposedPorts: []string{"5432/tcp"},
        Env: map[string]string{
            "POSTGRES_USER":     "test",
            "POSTGRES_PASSWORD": "test",
            "POSTGRES_DB":       "testdb",
        },
        WaitingFor: wait.ForListeningPort("5432/tcp"),
    }
    
    container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: req,
        Started:          true,
    })
    suite.Require().NoError(err)
    
    suite.container = container
    
    // Get connection string
    host, err := container.Host(ctx)
    suite.Require().NoError(err)
    
    port, err := container.MappedPort(ctx, "5432")
    suite.Require().NoError(err)
    
    dsn := fmt.Sprintf("postgres://test:test@%s:%s/testdb?sslmode=disable", host, port.Port())
    
    // Connect to database
    db, err := sql.Open("postgres", dsn)
    suite.Require().NoError(err)
    
    suite.db = db
    suite.repo = NewPostgresUserRepository(db)
    
    // Run migrations
    suite.runMigrations()
}

func (suite *UserRepositoryTestSuite) TearDownSuite() {
    suite.db.Close()
    suite.container.Terminate(context.Background())
}

func (suite *UserRepositoryTestSuite) TestCreate() {
    user := &User{
        ID:    "test-123",
        Email: "test@example.com",
        Name:  "Test User",
    }
    
    err := suite.repo.Create(context.Background(), user)
    suite.NoError(err)
    
    // Verify
    got, err := suite.repo.GetByID(context.Background(), user.ID)
    suite.NoError(err)
    suite.Equal(user.Email, got.Email)
}

func TestUserRepository(t *testing.T) {
    suite.Run(t, new(UserRepositoryTestSuite))
}
```

## API Testing

### HTTP Handler Testing
```go
// internal/handlers/user_test.go
package handlers

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    
    "github.com/stretchr/testify/assert"
    "github.com/gorilla/mux"
)

func TestUserHandler_Create(t *testing.T) {
    tests := []struct {
        name       string
        input      interface{}
        setup      func(*mocks.UserService)
        wantStatus int
        wantBody   interface{}
    }{
        {
            name: "successful creation",
            input: CreateUserRequest{
                Email:    "new@example.com",
                Name:     "New User",
                Password: "password123",
            },
            setup: func(m *mocks.UserService) {
                m.On("Create", mock.Anything, mock.AnythingOfType("*handlers.CreateUserRequest")).
                    Return(&User{
                        ID:    "123",
                        Email: "new@example.com",
                        Name:  "New User",
                    }, nil)
            },
            wantStatus: http.StatusCreated,
            wantBody: map[string]interface{}{
                "id":    "123",
                "email": "new@example.com",
                "name":  "New User",
            },
        },
        {
            name: "validation error",
            input: CreateUserRequest{
                Email: "invalid-email",
            },
            setup: func(m *mocks.UserService) {
                // No service call expected
            },
            wantStatus: http.StatusBadRequest,
            wantBody: map[string]interface{}{
                "error": "validation failed",
            },
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup
            svc := new(mocks.UserService)
            if tt.setup != nil {
                tt.setup(svc)
            }
            
            handler := NewUserHandler(svc)
            
            // Create request
            body, _ := json.Marshal(tt.input)
            req := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBuffer(body))
            req.Header.Set("Content-Type", "application/json")
            
            // Create response recorder
            rr := httptest.NewRecorder()
            
            // Execute
            handler.Create(rr, req)
            
            // Assert
            assert.Equal(t, tt.wantStatus, rr.Code)
            
            if tt.wantBody != nil {
                var got interface{}
                json.Unmarshal(rr.Body.Bytes(), &got)
                assert.Equal(t, tt.wantBody, got)
            }
            
            svc.AssertExpectations(t)
        })
    }
}
```

## End-to-End Testing

```go
// tests/e2e/user_test.go
package e2e

import (
    "bytes"
    "encoding/json"
    "net/http"
    "testing"
    "time"
    
    "github.com/stretchr/testify/suite"
)

type E2ETestSuite struct {
    suite.Suite
    baseURL string
    client  *http.Client
}

func (suite *E2ETestSuite) SetupSuite() {
    suite.baseURL = "http://localhost:8080"
    suite.client = &http.Client{
        Timeout: 10 * time.Second,
    }
    
    // Wait for server to be ready
    suite.waitForServer()
}

func (suite *E2ETestSuite) TestUserLifecycle() {
    // 1. Register new user
    registerReq := map[string]string{
        "email":    "e2e@example.com",
        "password": "password123",
        "name":     "E2E User",
    }
    
    resp := suite.makeRequest("POST", "/api/v1/auth/register", registerReq)
    suite.Equal(http.StatusCreated, resp.StatusCode)
    
    var registerResp map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&registerResp)
    
    // 2. Login
    loginReq := map[string]string{
        "email":    "e2e@example.com",
        "password": "password123",
    }
    
    resp = suite.makeRequest("POST", "/api/v1/auth/login", loginReq)
    suite.Equal(http.StatusOK, resp.StatusCode)
    
    var loginResp map[string]string
    json.NewDecoder(resp.Body).Decode(&loginResp)
    token := loginResp["token"]
    
    // 3. Get profile
    req, _ := http.NewRequest("GET", suite.baseURL+"/api/v1/users/me", nil)
    req.Header.Set("Authorization", "Bearer "+token)
    
    resp, err := suite.client.Do(req)
    suite.NoError(err)
    suite.Equal(http.StatusOK, resp.StatusCode)
}

func (suite *E2ETestSuite) makeRequest(method, path string, body interface{}) *http.Response {
    var bodyReader *bytes.Reader
    if body != nil {
        bodyBytes, _ := json.Marshal(body)
        bodyReader = bytes.NewReader(bodyBytes)
    }
    
    req, _ := http.NewRequest(method, suite.baseURL+path, bodyReader)
    if body != nil {
        req.Header.Set("Content-Type", "application/json")
    }
    
    resp, err := suite.client.Do(req)
    suite.NoError(err)
    
    return resp
}
```

## Benchmark Testing

```go
// internal/services/user_bench_test.go
package services

import (
    "context"
    "testing"
)

func BenchmarkUserService_GetByID(b *testing.B) {
    repo := new(mocks.UserRepository)
    repo.On("GetByID", mock.Anything, "123").Return(&User{
        ID:    "123",
        Email: "bench@example.com",
        Name:  "Bench User",
    }, nil)
    
    svc := NewUserService(repo)
    ctx := context.Background()
    
    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            _, _ = svc.GetByID(ctx, "123")
        }
    })
}

func BenchmarkUserService_List(b *testing.B) {
    users := make([]*User, 100)
    for i := range users {
        users[i] = &User{
            ID:    fmt.Sprintf("user-%d", i),
            Email: fmt.Sprintf("user%d@example.com", i),
            Name:  fmt.Sprintf("User %d", i),
        }
    }
    
    repo := new(mocks.UserRepository)
    repo.On("List", mock.Anything, mock.Anything).Return(users, nil)
    
    svc := NewUserService(repo)
    ctx := context.Background()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = svc.List(ctx, ListFilter{Limit: 100})
    }
}
```

## Test Helpers and Utilities

```go
// tests/helpers/database.go
package helpers

func SetupTestDB(t *testing.T) (*sql.DB, func()) {
    db, err := sql.Open("postgres", "postgres://test:test@localhost:5432/testdb?sslmode=disable")
    require.NoError(t, err)
    
    // Run migrations
    err = migrate.Up(db, "file://../../migrations")
    require.NoError(t, err)
    
    cleanup := func() {
        db.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
        db.Close()
    }
    
    return db, cleanup
}

// tests/helpers/fixtures.go
package helpers

func CreateTestUser(t *testing.T, db *sql.DB) *User {
    user := &User{
        ID:    uuid.New().String(),
        Email: fmt.Sprintf("test-%s@example.com", uuid.New().String()),
        Name:  "Test User",
    }
    
    _, err := db.Exec(
        "INSERT INTO users (id, email, name) VALUES ($1, $2, $3)",
        user.ID, user.Email, user.Name,
    )
    require.NoError(t, err)
    
    return user
}
```

## Testing Best Practices

1. **Use table-driven tests** for comprehensive coverage
2. **Mock external dependencies** in unit tests
3. **Use TestContainers** for integration tests
4. **Write benchmarks** for performance-critical code
5. **Test error cases** thoroughly
6. **Use test fixtures** for consistent test data
7. **Parallelize tests** where possible
8. **Keep tests independent** and idempotent
9. **Use coverage tools** but aim for quality over quantity
10. **Test concurrency** with race detector enabled

## Running Tests

```bash
# Unit tests
go test ./...

# With coverage
go test -cover ./...

# Integration tests
go test -tags=integration ./...

# Benchmarks
go test -bench=. -benchmem ./...

# Race detection
go test -race ./...

# Specific test
go test -run TestUserService_Create ./internal/services
```

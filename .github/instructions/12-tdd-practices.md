# TDD Practices for Go API Development

## Core TDD Principles

### The Three Laws of TDD

1. **You may not write production code until you have written a failing unit test**
2. **You may not write more of a unit test than is sufficient to fail**
3. **You may not write more production code than is sufficient to pass the current failing test**

## Red-Green-Refactor Cycle

### 1. RED Phase - Write a Failing Test

```go
// Example: Testing a calculator service
func TestCalculator_Add(t *testing.T) {
    calc := NewCalculator()
    result := calc.Add(2, 3)
    assert.Equal(t, 5, result)
}
```

### 2. GREEN Phase - Make the Test Pass

```go
type Calculator struct{}

func NewCalculator() *Calculator {
    return &Calculator{}
}

func (c *Calculator) Add(a, b int) int {
    return a + b // Simplest implementation that works
}
```

### 3. REFACTOR Phase - Improve the Code

```go
// After multiple test cases, refactor for better design
type Calculator struct {
    history []Operation
}

type Operation struct {
    Type   string
    Values []int
    Result int
}

func (c *Calculator) Add(a, b int) int {
    result := a + b
    c.history = append(c.history, Operation{
        Type:   "add",
        Values: []int{a, b},
        Result: result,
    })
    return result
}
```

## Testing Patterns

### Table-Driven Tests

```go
func TestUserValidator_Validate(t *testing.T) {
    tests := []struct {
        name    string
        input   User
        wantErr bool
        errMsg  string
    }{
        {
            name: "valid user",
            input: User{
                Email:    "john@example.com",
                Password: "secure123",
                Age:      25,
            },
            wantErr: false,
        },
        {
            name: "invalid email",
            input: User{
                Email:    "invalid-email",
                Password: "secure123",
                Age:      25,
            },
            wantErr: true,
            errMsg:  "invalid email format",
        },
        {
            name: "password too short",
            input: User{
                Email:    "john@example.com",
                Password: "123",
                Age:      25,
            },
            wantErr: true,
            errMsg:  "password must be at least 8 characters",
        },
    }

    validator := NewUserValidator()
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := validator.Validate(tt.input)
            if tt.wantErr {
                assert.Error(t, err)
                assert.Contains(t, err.Error(), tt.errMsg)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

### Testing HTTP Handlers

```go
func TestUserHandler_Create(t *testing.T) {
    t.Run("successful creation", func(t *testing.T) {
        // Arrange
        mockService := new(MockUserService)
        handler := NewUserHandler(mockService)
        
        payload := `{"email":"john@example.com","password":"secure123"}`
        req := httptest.NewRequest("POST", "/users", strings.NewReader(payload))
        req.Header.Set("Content-Type", "application/json")
        rec := httptest.NewRecorder()
        
        expectedUser := &User{ID: "123", Email: "john@example.com"}
        mockService.On("Create", mock.Anything).Return(expectedUser, nil)
        
        // Act
        handler.Create(rec, req)
        
        // Assert
        assert.Equal(t, http.StatusCreated, rec.Code)
        
        var response User
        json.Unmarshal(rec.Body.Bytes(), &response)
        assert.Equal(t, expectedUser.ID, response.ID)
        assert.Equal(t, expectedUser.Email, response.Email)
    })
}
```

### Testing with Interfaces

```go
// Define interface for testability
type UserRepository interface {
    Create(user *User) error
    GetByID(id string) (*User, error)
    GetByEmail(email string) (*User, error)
    Update(user *User) error
    Delete(id string) error
}

// Mock implementation for tests
type MockUserRepository struct {
    mock.Mock
}

func (m *MockUserRepository) Create(user *User) error {
    args := m.Called(user)
    return args.Error(0)
}

// Service using the interface
type UserService struct {
    repo UserRepository
}

func TestUserService_GetUserProfile(t *testing.T) {
    // Easy to test with mock
    mockRepo := new(MockUserRepository)
    service := NewUserService(mockRepo)
    
    expectedUser := &User{ID: "123", Name: "John"}
    mockRepo.On("GetByID", "123").Return(expectedUser, nil)
    
    user, err := service.GetUserProfile("123")
    
    assert.NoError(t, err)
    assert.Equal(t, expectedUser, user)
    mockRepo.AssertExpectations(t)
}
```

### Testing Async Operations

```go
func TestMessageQueue_ProcessAsync(t *testing.T) {
    t.Run("processes message asynchronously", func(t *testing.T) {
        // Arrange
        queue := NewMessageQueue()
        processed := make(chan Message, 1)
        
        handler := func(msg Message) error {
            processed <- msg
            return nil
        }
        
        queue.RegisterHandler(handler)
        message := Message{ID: "123", Body: "test"}
        
        // Act
        err := queue.Send(message)
        
        // Assert
        assert.NoError(t, err)
        
        select {
        case receivedMsg := <-processed:
            assert.Equal(t, message.ID, receivedMsg.ID)
        case <-time.After(2 * time.Second):
            t.Fatal("message not processed within timeout")
        }
    })
}
```

## Testing Database Operations

### Using Test Containers

```go
func TestUserRepository_PostgreSQL(t *testing.T) {
    ctx := context.Background()
    
    // Start PostgreSQL container
    container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: testcontainers.ContainerRequest{
            Image:        "postgres:13",
            ExposedPorts: []string{"5432/tcp"},
            Env: map[string]string{
                "POSTGRES_PASSWORD": "testpass",
                "POSTGRES_DB":       "testdb",
            },
            WaitingFor: wait.ForListeningPort("5432/tcp"),
        },
        Started: true,
    })
    require.NoError(t, err)
    defer container.Terminate(ctx)
    
    // Get connection details
    host, err := container.Host(ctx)
    require.NoError(t, err)
    
    port, err := container.MappedPort(ctx, "5432")
    require.NoError(t, err)
    
    // Connect to database
    dsn := fmt.Sprintf("host=%s port=%s user=postgres password=testpass dbname=testdb sslmode=disable", 
        host, port.Port())
    db, err := sql.Open("postgres", dsn)
    require.NoError(t, err)
    
    // Run migrations
    RunMigrations(db)
    
    // Test repository
    repo := NewUserRepository(db)
    
    t.Run("Create and Get User", func(t *testing.T) {
        user := &User{
            Email: "test@example.com",
            Name:  "Test User",
        }
        
        err := repo.Create(user)
        assert.NoError(t, err)
        assert.NotEmpty(t, user.ID)
        
        retrieved, err := repo.GetByID(user.ID)
        assert.NoError(t, err)
        assert.Equal(t, user.Email, retrieved.Email)
    })
}
```

### Using In-Memory Database

```go
func setupTestDB(t *testing.T) *gorm.DB {
    db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
    require.NoError(t, err)
    
    // Run migrations
    err = db.AutoMigrate(&User{}, &Order{})
    require.NoError(t, err)
    
    return db
}

func TestOrderRepository_CRUD(t *testing.T) {
    db := setupTestDB(t)
    repo := NewOrderRepository(db)
    
    t.Run("Create Order", func(t *testing.T) {
        order := &Order{
            UserID: "user123",
            Total:  99.99,
            Status: "pending",
        }
        
        err := repo.Create(order)
        assert.NoError(t, err)
        assert.NotZero(t, order.ID)
    })
}
```

## Testing Error Scenarios

```go
func TestPaymentService_ProcessPayment(t *testing.T) {
    tests := []struct {
        name          string
        amount        float64
        mockResponse  error
        expectedError string
    }{
        {
            name:          "successful payment",
            amount:        100.00,
            mockResponse:  nil,
            expectedError: "",
        },
        {
            name:          "insufficient funds",
            amount:        1000.00,
            mockResponse:  ErrInsufficientFunds,
            expectedError: "insufficient funds",
        },
        {
            name:          "payment gateway error",
            amount:        100.00,
            mockResponse:  errors.New("gateway timeout"),
            expectedError: "payment processing failed",
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            mockGateway := new(MockPaymentGateway)
            service := NewPaymentService(mockGateway)
            
            mockGateway.On("Charge", tt.amount).Return(tt.mockResponse)
            
            err := service.ProcessPayment(tt.amount)
            
            if tt.expectedError != "" {
                assert.Error(t, err)
                assert.Contains(t, err.Error(), tt.expectedError)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

## Test Organization

### Directory Structure

```
project/
├── internal/
│   ├── user/
│   │   ├── service.go
│   │   ├── service_test.go
│   │   ├── repository.go
│   │   ├── repository_test.go
│   │   └── testdata/
│   │       └── fixtures.go
│   └── order/
│       ├── service.go
│       └── service_test.go
├── pkg/
│   └── testutil/
│       ├── db.go
│       ├── fixtures.go
│       └── mocks.go
└── test/
    ├── integration/
    │   └── api_test.go
    └── e2e/
        └── user_flow_test.go
```

### Test Helpers

```go
// testutil/fixtures.go
package testutil

func NewTestUser(opts ...UserOption) *User {
    user := &User{
        ID:       uuid.New().String(),
        Email:    "test@example.com",
        Password: "hashed_password",
        Name:     "Test User",
    }
    
    for _, opt := range opts {
        opt(user)
    }
    
    return user
}

type UserOption func(*User)

func WithEmail(email string) UserOption {
    return func(u *User) {
        u.Email = email
    }
}

func WithName(name string) UserOption {
    return func(u *User) {
        u.Name = name
    }
}

// Usage in tests
func TestSomething(t *testing.T) {
    user := testutil.NewTestUser(
        testutil.WithEmail("custom@example.com"),
        testutil.WithName("Custom Name"),
    )
    // Use user in test...
}
```

## Continuous Testing

### Git Hooks

```bash
# .git/hooks/pre-commit
#!/bin/bash
echo "Running tests before commit..."
go test ./... -short
if [ $? -ne 0 ]; then
    echo "Tests failed. Commit aborted."
    exit 1
fi
```

### Makefile for Testing

```makefile
.PHONY: test test-unit test-integration test-e2e test-all coverage

test-unit:
	go test ./... -short -v

test-integration:
	go test ./test/integration/... -v

test-e2e:
	go test ./test/e2e/... -v

test-all: test-unit test-integration test-e2e

coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	open coverage.html

test-watch:
	reflex -r '\.go$$' -s -- sh -c 'clear && go test ./... -v'
```

## Next Steps

Continue to [Advanced Testing](./13-advanced-testing.md) for mutation testing and BDD implementation.

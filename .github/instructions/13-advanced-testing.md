# Advanced Testing: Mutation Testing and BDD with Cucumber

## Mutation Testing

### What is Mutation Testing?

Mutation testing evaluates the quality of your test suite by introducing small changes (mutations) to your code and checking if your tests detect these changes.

### Setting Up go-mutesting

```bash
# Install go-mutesting
go install github.com/zimmski/go-mutesting/cmd/go-mutesting@latest

# Basic usage
go-mutesting ./...

# With specific options
go-mutesting --debug --verbose ./pkg/...
```

### Common Mutations

1. **Conditional Boundary Mutations**
```go
// Original
if x > 10 {
    return true
}

// Mutations
if x >= 10 { // Changed > to >=
if x < 10 {  // Changed > to <
if x == 10 { // Changed > to ==
```

2. **Arithmetic Operator Mutations**
```go
// Original
result := a + b

// Mutations
result := a - b
result := a * b
result := a / b
```

3. **Boolean Mutations**
```go
// Original
if condition && otherCondition {

// Mutations
if condition || otherCondition {
if !condition && otherCondition {
```

### Writing Mutation-Resistant Tests

```go
// Weak test - won't catch boundary mutations
func TestIsAdult_Weak(t *testing.T) {
    assert.True(t, IsAdult(20))
    assert.False(t, IsAdult(10))
}

// Strong test - catches boundary mutations
func TestIsAdult_Strong(t *testing.T) {
    tests := []struct {
        name string
        age  int
        want bool
    }{
        {"exactly 18", 18, true},      // Boundary case
        {"just under 18", 17, false},  // Boundary - 1
        {"just over 18", 19, true},    // Boundary + 1
        {"negative age", -1, false},   // Edge case
        {"zero age", 0, false},        // Edge case
        {"very old", 100, true},       // Normal case
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            assert.Equal(t, tt.want, IsAdult(tt.age))
        })
    }
}
```

### Mutation Testing Configuration

```yaml
# .mutesting.yml
blacklist:
  - "generated"
  - "vendor"
  - "mocks"

mutators:
  - conditionals/negation
  - conditionals/boundary
  - arithmetic
  - increment-decrement
  - invert-negatives

timeout: 10s
```

### Analyzing Mutation Results

```bash
# Run mutation testing with detailed output
go-mutesting --verbose ./service/... | tee mutation-report.txt

# Example output analysis
# PASS: mutation at service/user.go:45:10 (changed > to >=) - caught by tests ✓
# FAIL: mutation at service/user.go:67:15 (removed nil check) - not caught ✗

# Calculate mutation score
# Mutation Score = (Killed Mutants / Total Mutants) * 100
```

## BDD with Cucumber (Godog)

### Setting Up Godog

```bash
# Install godog
go install github.com/cucumber/godog/cmd/godog@latest

# Initialize godog in your project
godog init
```

### Project Structure for BDD

```
project/
├── features/
│   ├── user_management.feature
│   ├── order_processing.feature
│   └── step_definitions/
│       ├── user_steps.go
│       ├── order_steps.go
│       └── common_steps.go
├── test/
│   └── features_test.go
└── ...
```

### Writing Feature Files

```gherkin
# features/user_management.feature
Feature: User Management
  In order to manage application users
  As an administrator
  I need to be able to create, update, and delete users

  Background:
    Given the system is initialized
    And I am authenticated as an admin

  Scenario: Create a new user
    Given I have the following user details:
      | field    | value              |
      | email    | john@example.com   |
      | name     | John Doe           |
      | role     | user               |
    When I send a POST request to "/api/users"
    Then the response status should be 201
    And the response should contain:
      """
      {
        "email": "john@example.com",
        "name": "John Doe",
        "role": "user"
      }
      """
    And a user with email "john@example.com" should exist in the database

  Scenario: Prevent duplicate user creation
    Given a user exists with email "existing@example.com"
    When I try to create a user with email "existing@example.com"
    Then the response status should be 409
    And the response should contain error "email already exists"

  Scenario Outline: Validate user input
    When I try to create a user with <field> "<value>"
    Then the response status should be 400
    And the response should contain error "<error>"

    Examples:
      | field    | value        | error                    |
      | email    |              | email is required        |
      | email    | invalid      | invalid email format     |
      | name     |              | name is required         |
      | password | short        | password too short       |
```

### Implementing Step Definitions

```go
// features/step_definitions/user_steps.go
package steps

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "net/http/httptest"
    
    "github.com/cucumber/godog"
    "github.com/stretchr/testify/assert"
)

type UserFeature struct {
    server       *httptest.Server
    response     *http.Response
    responseBody []byte
    userDetails  map[string]interface{}
    testDB       *TestDatabase
}

func (u *UserFeature) iHaveTheFollowingUserDetails(table *godog.Table) error {
    u.userDetails = make(map[string]interface{})
    
    for _, row := range table.Rows[1:] { // Skip header
        field := row.Cells[0].Value
        value := row.Cells[1].Value
        u.userDetails[field] = value
    }
    
    return nil
}

func (u *UserFeature) iSendAPOSTRequestTo(endpoint string) error {
    body, err := json.Marshal(u.userDetails)
    if err != nil {
        return err
    }
    
    req, err := http.NewRequest("POST", u.server.URL+endpoint, bytes.NewBuffer(body))
    if err != nil {
        return err
    }
    
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer "+u.authToken)
    
    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    
    u.response = resp
    u.responseBody, _ = ioutil.ReadAll(resp.Body)
    resp.Body.Close()
    
    return nil
}

func (u *UserFeature) theResponseStatusShouldBe(expectedStatus int) error {
    if u.response.StatusCode != expectedStatus {
        return fmt.Errorf("expected status %d, got %d", expectedStatus, u.response.StatusCode)
    }
    return nil
}

func (u *UserFeature) theResponseShouldContain(expectedJSON *godog.DocString) error {
    var expected, actual map[string]interface{}
    
    if err := json.Unmarshal([]byte(expectedJSON.Content), &expected); err != nil {
        return err
    }
    
    if err := json.Unmarshal(u.responseBody, &actual); err != nil {
        return err
    }
    
    for key, expectedValue := range expected {
        if actualValue, ok := actual[key]; !ok || actualValue != expectedValue {
            return fmt.Errorf("expected %s to be %v, got %v", key, expectedValue, actualValue)
        }
    }
    
    return nil
}

func (u *UserFeature) aUserWithEmailShouldExistInTheDatabase(email string) error {
    user, err := u.testDB.GetUserByEmail(email)
    if err != nil {
        return fmt.Errorf("failed to find user: %w", err)
    }
    
    if user == nil {
        return fmt.Errorf("user with email %s does not exist", email)
    }
    
    return nil
}
```

### Test Runner Setup

```go
// test/features_test.go
package test

import (
    "os"
    "testing"
    
    "github.com/cucumber/godog"
    "github.com/cucumber/godog/colors"
    "myapp/features/steps"
)

var opts = godog.Options{
    Format:      "pretty",
    Paths:       []string{"../features"},
    Randomize:   -1, // Randomize scenario execution order
    Concurrency: 4,  // Run scenarios in parallel
}

func init() {
    godog.BindCommandLineFlags("godog.", &opts)
}

func TestFeatures(t *testing.T) {
    o := opts
    o.TestingT = t

    status := godog.TestSuite{
        Name:                "user-management",
        ScenarioInitializer: InitializeScenario,
        Options:             &o,
    }.Run()

    if status != 0 {
        t.Fail()
    }
}

func InitializeScenario(ctx *godog.ScenarioContext) {
    userFeature := &steps.UserFeature{}
    
    // Before scenario hooks
    ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
        // Setup test database
        userFeature.testDB = steps.NewTestDatabase()
        
        // Start test server
        userFeature.server = httptest.NewServer(app.Handler())
        
        return ctx, nil
    })
    
    // After scenario hooks
    ctx.After(func(ctx context.Context, sc *godog.Scenario, err error) (context.Context, error) {
        // Cleanup
        userFeature.server.Close()
        userFeature.testDB.Cleanup()
        
        return ctx, nil
    })
    
    // Step definitions
    ctx.Step(`^the system is initialized$`, userFeature.theSystemIsInitialized)
    ctx.Step(`^I am authenticated as an admin$`, userFeature.iAmAuthenticatedAsAdmin)
    ctx.Step(`^I have the following user details:$`, userFeature.iHaveTheFollowingUserDetails)
    ctx.Step(`^I send a POST request to "([^"]*)"$`, userFeature.iSendAPOSTRequestTo)
    ctx.Step(`^the response status should be (\d+)$`, userFeature.theResponseStatusShouldBe)
    ctx.Step(`^the response should contain:$`, userFeature.theResponseShouldContain)
    ctx.Step(`^a user with email "([^"]*)" should exist in the database$`, userFeature.aUserWithEmailShouldExistInTheDatabase)
}
```

### Advanced BDD Patterns

#### 1. Shared Context

```go
// features/step_definitions/context.go
package steps

type ScenarioContext struct {
    User         *User
    Response     *http.Response
    ResponseBody []byte
    Database     *TestDatabase
    Server       *httptest.Server
    AuthToken    string
}

func NewScenarioContext() *ScenarioContext {
    return &ScenarioContext{
        Database: NewTestDatabase(),
    }
}
```

#### 2. Background Steps

```go
func (c *ScenarioContext) givenIAmLoggedInAs(role string) error {
    user := &User{
        Email:    fmt.Sprintf("%s@example.com", role),
        Password: "password123",
        Role:     role,
    }
    
    if err := c.Database.CreateUser(user); err != nil {
        return err
    }
    
    token, err := generateAuthToken(user)
    if err != nil {
        return err
    }
    
    c.AuthToken = token
    c.User = user
    
    return nil
}
```

#### 3. Data Tables and Examples

```go
func (c *ScenarioContext) iCreateMultipleUsers(users *godog.Table) error {
    for i, row := range users.Rows {
        if i == 0 { // Skip header
            continue
        }
        
        user := &User{
            Email: row.Cells[0].Value,
            Name:  row.Cells[1].Value,
            Role:  row.Cells[2].Value,
        }
        
        if err := c.Database.CreateUser(user); err != nil {
            return fmt.Errorf("failed to create user %s: %w", user.Email, err)
        }
    }
    
    return nil
}
```

### Running BDD Tests

```bash
# Run all features
godog run

# Run specific feature
godog run features/user_management.feature

# Run with specific tags
godog run --tags=@critical

# Generate HTML report
godog run --format=html > report.html

# Run in parallel
godog run --concurrency=4
```

### Integration with CI/CD

```yaml
# .github/workflows/test.yml
name: Test

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    
    steps:
    - uses: actions/checkout@v2
    
    - name: Set up Go
      uses: actions/setup-go@v2
      with:
        go-version: 1.19
    
    - name: Install dependencies
      run: |
        go mod download
        go install github.com/zimmski/go-mutesting/cmd/go-mutesting@latest
        go install github.com/cucumber/godog/cmd/godog@latest
    
    - name: Run unit tests
      run: go test ./... -v -coverprofile=coverage.out
    
    - name: Run mutation tests
      run: go-mutesting ./... || true
    
    - name: Run BDD tests
      run: godog run --format=junit > bdd-report.xml
    
    - name: Upload test results
      uses: actions/upload-artifact@v2
      with:
        name: test-results
        path: |
          coverage.out
          bdd-report.xml
```

## Complete Testing Workflow

### Makefile for Complete Workflow

```makefile
.PHONY: test-workflow

test-workflow:
	@echo "🔵 Starting TDD Workflow..."
	@echo "1️⃣ Running unit tests..."
	@go test ./... -v -short || (echo "❌ Unit tests failed" && exit 1)
	@echo "✅ Unit tests passed"
	
	@echo "2️⃣ Running integration tests..."
	@go test ./test/integration/... -v || (echo "❌ Integration tests failed" && exit 1)
	@echo "✅ Integration tests passed"
	
	@echo "3️⃣ Running mutation tests..."
	@go-mutesting ./... --verbose || true
	@echo "✅ Mutation testing complete"
	
	@echo "4️⃣ Running BDD tests..."
	@godog run || (echo "❌ BDD tests failed" && exit 1)
	@echo "✅ BDD tests passed"
	
	@echo "5️⃣ Checking coverage..."
	@go test ./... -coverprofile=coverage.out
	@go tool cover -func=coverage.out | grep total | awk '{print "Total Coverage: " $$3}'
	
	@echo "🎉 All tests passed! Ready to commit."

pre-commit: test-workflow
	@echo "Running pre-commit checks..."
	@golangci-lint run
	@go fmt ./...
	@go mod tidy
```

## Best Practices Summary

1. **Write tests first** - Always start with a failing test
2. **Test boundaries** - Focus on edge cases and boundaries
3. **Use mutation testing** - Ensure your tests are effective
4. **Write scenarios in business language** - BDD features should be readable by non-developers
5. **Keep tests independent** - Each test should be able to run in isolation
6. **Use proper test data** - Create realistic test scenarios
7. **Clean up after tests** - Always clean up test data and resources
8. **Monitor test performance** - Keep tests fast
9. **Review test coverage** - Aim for high coverage but focus on quality
10. **Automate everything** - Integrate all tests into CI/CD pipeline

# TDD Workflow for Go API Development

## Overview

This document outlines a comprehensive Test-Driven Development (TDD) workflow for building Go APIs, incorporating unit tests, mutation testing, and BDD with Cucumber.

## Prerequisites

### Required Tools

```bash
# Install Go
brew install go

# Install testing tools
go install github.com/onsi/ginkgo/v2/ginkgo@latest
go install github.com/onsi/gomega/...@latest
go install github.com/zimmski/go-mutesting/cmd/go-mutesting@latest
go install github.com/cucumber/godog/cmd/godog@latest

# Install code quality tools
go install golang.org/x/tools/cmd/goimports@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

## Workflow Steps

### 1. Feature Planning

Before writing any code:

```bash
# Create feature branch
git checkout -b feature/user-registration

# Create feature file for BDD
mkdir -p features
cat > features/user_registration.feature << EOF
Feature: User Registration
  As a new user
  I want to register an account
  So that I can access the application

  Scenario: Successful registration
    Given I have valid registration details
    When I submit the registration form
    Then I should receive a success response
    And a new user should be created in the system

  Scenario: Duplicate email registration
    Given a user exists with email "john@example.com"
    When I try to register with email "john@example.com"
    Then I should receive an error "email already exists"
EOF
```

### 2. Write Failing Test (RED Phase)

Start with a failing unit test:

```go
// user_service_test.go
package service_test

import (
    "testing"
    "myapp/service"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
)

func TestUserService_Register(t *testing.T) {
    t.Run("should register new user successfully", func(t *testing.T) {
        // Arrange
        mockRepo := new(MockUserRepository)
        userService := service.NewUserService(mockRepo)
        
        input := service.RegisterInput{
            Email:    "john@example.com",
            Password: "securepass123",
            Name:     "John Doe",
        }
        
        mockRepo.On("GetByEmail", input.Email).Return(nil, nil)
        mockRepo.On("Create", mock.Anything).Return(nil)
        
        // Act
        user, err := userService.Register(input)
        
        // Assert
        assert.NoError(t, err)
        assert.NotNil(t, user)
        assert.Equal(t, input.Email, user.Email)
        mockRepo.AssertExpectations(t)
    })
}
```

### 3. Run Test and Verify Failure

```bash
# Run the test
go test ./service -v

# Expected output: FAIL (code doesn't exist yet)
```

### 4. Write Minimal Code (GREEN Phase)

Implement just enough code to pass the test:

```go
// user_service.go
package service

type UserService struct {
    repo UserRepository
}

func NewUserService(repo UserRepository) *UserService {
    return &UserService{repo: repo}
}

func (s *UserService) Register(input RegisterInput) (*User, error) {
    // Check if user exists
    existing, err := s.repo.GetByEmail(input.Email)
    if err != nil {
        return nil, err
    }
    if existing != nil {
        return nil, errors.New("email already exists")
    }
    
    // Create user
    user := &User{
        Email:    input.Email,
        Password: hashPassword(input.Password),
        Name:     input.Name,
    }
    
    if err := s.repo.Create(user); err != nil {
        return nil, err
    }
    
    return user, nil
}
```

### 5. Run Tests Again

```bash
# Run unit tests
go test ./service -v

# Run with coverage
go test ./service -v -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### 6. Refactor

Improve code quality while keeping tests green:

```bash
# Format code
goimports -w .

# Run linter
golangci-lint run

# Run tests again
go test ./... -v
```

### 7. Run Mutation Testing

```bash
# Run mutation tests
go-mutesting ./service/...

# Analyze results and add tests for surviving mutants
```

### 8. Implement BDD Step Definitions

```go
// features/user_registration_test.go
package features

import (
    "github.com/cucumber/godog"
)

func InitializeScenario(ctx *godog.ScenarioContext) {
    ctx.Step(`^I have valid registration details$`, iHaveValidRegistrationDetails)
    ctx.Step(`^I submit the registration form$`, iSubmitTheRegistrationForm)
    ctx.Step(`^I should receive a success response$`, iShouldReceiveSuccessResponse)
    ctx.Step(`^a new user should be created in the system$`, userShouldBeCreated)
}

func TestFeatures(t *testing.T) {
    suite := godog.TestSuite{
        ScenarioInitializer: InitializeScenario,
        Options: &godog.Options{
            Format:   "pretty",
            Paths:    []string{"features"},
            TestingT: t,
        },
    }

    if suite.Run() != 0 {
        t.Fatal("non-zero status returned, failed to run feature tests")
    }
}
```

### 9. Run BDD Tests

```bash
# Run Cucumber tests
go test -v ./features/...
```

### 10. Commit and Push

```bash
# Run all tests one more time
go test ./... -v

# Check test coverage
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out

# If all tests pass and coverage is good
git add .
git commit -m "feat: implement user registration with TDD"
git push origin feature/user-registration
```

### 11. Error Handling Loop

If any test fails at any point:

```bash
# 1. Identify the failing test
go test ./... -v | grep FAIL

# 2. Fix the code
# 3. Re-run tests
go test ./... -v

# 4. If tests pass, continue to next step
# 5. If tests fail, repeat from step 2
```

## Continuous Workflow Script

Create a script to automate the workflow:

```bash
#!/bin/bash
# tdd-workflow.sh

set -e

echo "🔴 Running unit tests..."
if ! go test ./... -v; then
    echo "❌ Unit tests failed. Fix the code and try again."
    exit 1
fi

echo "🟢 Unit tests passed!"

echo "🧬 Running mutation tests..."
go-mutesting ./... || true

echo "🥒 Running BDD tests..."
if ! go test -v ./features/...; then
    echo "❌ BDD tests failed. Fix the code and try again."
    exit 1
fi

echo "📊 Generating coverage report..."
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out

echo "🎯 Running linter..."
golangci-lint run

echo "✅ All tests passed! Ready to commit."
```

## Best Practices

1. **Write One Test at a Time**: Focus on a single behavior
2. **Keep Tests Small**: Each test should verify one thing
3. **Use Descriptive Names**: Test names should explain what they test
4. **Follow AAA Pattern**: Arrange, Act, Assert
5. **Mock External Dependencies**: Keep tests isolated
6. **Refactor Regularly**: Clean code after tests pass
7. **Maintain High Coverage**: Aim for >80% code coverage
8. **Test Edge Cases**: Don't just test happy paths

## Coverage Goals

- Unit Test Coverage: >= 80%
- Mutation Test Score: >= 70%
- BDD Scenario Coverage: 100% of user stories
- Integration Test Coverage: All API endpoints

## Next Steps

Continue to [TDD Practices](./12-tdd-practices.md) for detailed implementation patterns.

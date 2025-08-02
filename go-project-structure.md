# Go Project Structure

This document describes the recommended structure for a Go project, including the use of hexagonal architecture (ports and adapters).

## General Structure

A typical Go project structure may look like this:

```
project-root/
├── cmd/
│   └── api/
│       └── main.go
├── internal/
│   ├── app/
│   │   ├── config/
│   │   ├── handler/
│   │   ├── middleware/
│   │   └── router/
│   └── domain/
│       ├── model/
│       ├── repository/
│       └── service/
├── pkg/
│   ├── logger/
│   └── utils/
├── scripts/
├── test/
├── go.mod
└── README.md
```

### Description of Directories:

- `cmd/`: Contains the entry points of the application.
- `internal/`: Contains application internal packages, not meant to be used by external applications.
- `pkg/`: Contains shared packages and utilities.
- `scripts/`: Contains scripts for automation.
- `test/`: Contains test data and test-related utilities.

## Hexagonal Architecture Structure

When implementing hexagonal architecture (ports and adapters), organize your project to clearly distinguish between incoming and outgoing adapters:

```
project-root/
├── cmd/
│   └── api/
│       └── main.go
├── internal/
│   ├── core/                 # Business logic (hexagon center)
│   │   ├── domain/          # Domain models
│   │   ├── ports/           # Interfaces (contracts)
│   │   │   ├── incoming/    # Input ports (use cases)
│   │   │   └── outgoing/    # Output ports (repositories)
│   │   └── services/        # Use case implementations
│   └── adapters/            # External adapters
│       ├── incoming/        # Driving adapters (API, CLI, etc.)
│       │   ├── http/        # HTTP REST handlers
│       │   ├── grpc/        # gRPC handlers
│       │   └── cli/         # CLI commands
│       └── outgoing/        # Driven adapters (DB, external services)
│           ├── database/    # Database implementations
│           │   ├── postgres/
│           │   └── mongodb/
│           ├── cache/       # Cache implementations
│           │   └── redis/
│           └── external/    # External service clients
│               ├── payment/
│               └── email/
├── pkg/                     # Shared utilities
└── go.mod
```

### Key Principles:

1. **Incoming Adapters** (Left side of hexagon):
   - Handle external requests
   - Convert external formats to domain models
   - Call input ports (use cases)

2. **Outgoing Adapters** (Right side of hexagon):
   - Implement output ports
   - Handle persistence and external communications
   - Convert domain models to external formats

3. **Core Domain**:
   - Contains business logic
   - Independent of external dependencies
   - Defines ports (interfaces) for adapters
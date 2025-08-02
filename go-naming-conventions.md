# Go Naming Conventions

## General Naming Conventions
- Use `CamelCase` for exported names (e.g., `CalculateTax`).
- Use `camelCase` for unexported names (e.g., `calculateTotal`).
- Use `UPPER_SNAKE_CASE` for constants (e.g., `PI`, `MAX_LENGTH`).
- Use `snake_case` for file names (e.g., `user_service.go`, `order_handler.go`).

## Package Naming
- Use short, meaningful names (e.g., `math`, `http`, `json`).
- Avoid using underscores or camel case (e.g., use `user` instead of `user_service`).

## Hexagonal Architecture Naming Conventions

### Folder Structure:
- Use `incoming/` for driving adapters (API handlers, CLI, etc.)
- Use `outgoing/` for driven adapters (databases, external services)
- Use `ports/` for interfaces
- Use `domain/` for business entities
- Use `services/` for use case implementations

### Interface Naming:
- Input ports: `<Entity>Service` (e.g., `UserService`, `OrderService`)
- Output ports: `<Entity>Repository` or `<Entity>Client` (e.g., `UserRepository`, `PaymentClient`)

### Implementation Naming:
- Incoming adapters: `<Protocol><Entity>Handler` (e.g., `HTTPUserHandler`, `GRPCOrderHandler`)
- Outgoing adapters: `<Technology><Entity>Repository` (e.g., `PostgresUserRepository`, `RedisCache`)
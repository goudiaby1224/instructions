# Database Operations Best Practices

## Database Connection Management

```go
// internal/database/postgres.go
package database

import (
    "database/sql"
    "fmt"
    "time"
    
    _ "github.com/lib/pq"
)

type Config struct {
    Host            string
    Port            int
    User            string
    Password        string
    DBName          string
    SSLMode         string
    MaxOpenConns    int
    MaxIdleConns    int
    ConnMaxLifetime time.Duration
}

func NewPostgresDB(cfg Config) (*sql.DB, error) {
    dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
        cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode)
    
    db, err := sql.Open("postgres", dsn)
    if err != nil {
        return nil, fmt.Errorf("failed to open database: %w", err)
    }
    
    // Connection pool settings
    db.SetMaxOpenConns(cfg.MaxOpenConns)
    db.SetMaxIdleConns(cfg.MaxIdleConns)
    db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
    
    // Verify connection
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    if err := db.PingContext(ctx); err != nil {
        return nil, fmt.Errorf("failed to ping database: %w", err)
    }
    
    return db, nil
}
```

## Repository Pattern with Hexagonal Architecture

### Port Definition

```go
// internal/core/ports/outgoing/user_repository.go
package outgoing

import (
    "context"
    "myapi/internal/core/domain"
)

type UserRepository interface {
    Save(ctx context.Context, user *domain.User) error
    FindByID(ctx context.Context, id string) (*domain.User, error)
    FindByEmail(ctx context.Context, email string) (*domain.User, error)
    FindAll(ctx context.Context) ([]*domain.User, error)
    Update(ctx context.Context, user *domain.User) error
    Delete(ctx context.Context, id string) error
}

type ListFilter struct {
    Limit  int
    Offset int
    Search string
}
```

### PostgreSQL Adapter Implementation

```go
// internal/adapters/outgoing/database/postgres/user_repository.go
package postgres

import (
    "context"
    "database/sql"
    "fmt"
    "myapi/internal/core/domain"
    "myapi/internal/core/ports/outgoing"
)

type userRepository struct {
    db *sql.DB
}

func NewUserRepository(db *sql.DB) outgoing.UserRepository {
    return &userRepository{db: db}
}

func (r *userRepository) Save(ctx context.Context, user *domain.User) error {
    query := `
        INSERT INTO users (id, email, name, password_hash, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6)
    `
    
    _, err := r.db.ExecContext(ctx, query,
        user.ID,
        user.Email,
        user.Name,
        user.PasswordHash,
        user.CreatedAt,
        user.UpdatedAt,
    )
    
    if err != nil {
        if isUniqueViolation(err) {
            return domain.ErrEmailAlreadyExists
        }
        return fmt.Errorf("failed to save user: %w", err)
    }
    
    return nil
}

func (r *userRepository) FindByID(ctx context.Context, id string) (*domain.User, error) {
    query := `
        SELECT id, email, name, password_hash, created_at, updated_at
        FROM users
        WHERE id = $1 AND deleted_at IS NULL
    `
    
    var user domain.User
    err := r.db.QueryRowContext(ctx, query, id).Scan(
        &user.ID,
        &user.Email,
        &user.Name,
        &user.PasswordHash,
        &user.CreatedAt,
        &user.UpdatedAt,
    )
    
    if err != nil {
        if err == sql.ErrNoRows {
            return nil, domain.ErrNotFound
        }
        return nil, fmt.Errorf("failed to find user: %w", err)
    }
    
    return &user, nil
}

func (r *userRepository) FindAll(ctx context.Context) ([]*domain.User, error) {
    query := `
        SELECT id, email, name, password_hash, created_at, updated_at
        FROM users
        WHERE deleted_at IS NULL
    `
    
    rows, err := r.db.QueryContext(ctx, query)
    if err != nil {
        return nil, fmt.Errorf("failed to list users: %w", err)
    }
    defer rows.Close()
    
    var users []*domain.User
    for rows.Next() {
        var user domain.User
        if err := rows.Scan(
            &user.ID,
            &user.Email,
            &user.Name,
            &user.PasswordHash,
            &user.CreatedAt,
            &user.UpdatedAt,
        ); err != nil {
            return nil, fmt.Errorf("failed to scan user: %w", err)
        }
        users = append(users, &user)
    }
    
    return users, nil
}
```

### MongoDB Adapter Implementation

```go
// internal/adapters/outgoing/database/mongodb/user_repository.go
package mongodb

import (
    "context"
    "myapi/internal/core/domain"
    "myapi/internal/core/ports/outgoing"
    "go.mongodb.org/mongo-driver/bson"
    "go.mongodb.org/mongo-driver/mongo"
)

type userRepository struct {
    collection *mongo.Collection
}

func NewUserRepository(db *mongo.Database) outgoing.UserRepository {
    return &userRepository{
        collection: db.Collection("users"),
    }
}

func (r *userRepository) Save(ctx context.Context, user *domain.User) error {
    doc := toDocument(user)
    _, err := r.collection.InsertOne(ctx, doc)
    if err != nil {
        if mongo.IsDuplicateKeyError(err) {
            return domain.ErrEmailAlreadyExists
        }
        return fmt.Errorf("failed to save user: %w", err)
    }
    return nil
}

func (r *userRepository) FindByID(ctx context.Context, id string) (*domain.User, error) {
    var doc userDocument
    err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&doc)
    if err != nil {
        if err == mongo.ErrNoDocuments {
            return nil, domain.ErrNotFound
        }
        return nil, fmt.Errorf("failed to find user: %w", err)
    }
    return toDomainUser(doc), nil
}

// Document mapping
type userDocument struct {
    ID           string    `bson:"_id"`
    Email        string    `bson:"email"`
    Name         string    `bson:"name"`
    PasswordHash string    `bson:"password_hash"`
    CreatedAt    time.Time `bson:"created_at"`
    UpdatedAt    time.Time `bson:"updated_at"`
}

func toDocument(user *domain.User) userDocument {
    return userDocument{
        ID:           user.ID,
        Email:        user.Email,
        Name:         user.Name,
        PasswordHash: user.PasswordHash,
        CreatedAt:    user.CreatedAt,
        UpdatedAt:    user.UpdatedAt,
    }
}

func toDomainUser(doc userDocument) *domain.User {
    return &domain.User{
        ID:           doc.ID,
        Email:        doc.Email,
        Name:         doc.Name,
        PasswordHash: doc.PasswordHash,
        CreatedAt:    doc.CreatedAt,
        UpdatedAt:    doc.UpdatedAt,
    }
}
```

## Transaction Management with Ports

```go
// internal/core/ports/outgoing/transaction.go
package outgoing

import "context"

type TransactionManager interface {
    ExecuteInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// internal/adapters/outgoing/database/postgres/transaction.go
package postgres

type transactionManager struct {
    db *sql.DB
}

func NewTransactionManager(db *sql.DB) outgoing.TransactionManager {
    return &transactionManager{db: db}
}

func (tm *transactionManager) ExecuteInTx(ctx context.Context, fn func(ctx context.Context) error) error {
    tx, err := tm.db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    
    defer func() {
        if p := recover(); p != nil {
            tx.Rollback()
            panic(p)
        }
    }()
    
    // Store transaction in context
    ctx = context.WithValue(ctx, "tx", tx)
    
    if err := fn(ctx); err != nil {
        tx.Rollback()
        return err
    }
    
    return tx.Commit()
}
```

## Using SQLX

```go
// internal/repository/user_sqlx.go
package repository

import "github.com/jmoiron/sqlx"

type sqlxUserRepository struct {
    db *sqlx.DB
}

func NewSQLXUserRepository(db *sqlx.DB) UserRepository {
    return &sqlxUserRepository{db: db}
}

func (r *sqlxUserRepository) GetByID(ctx context.Context, id string) (*User, error) {
    var user User
    query := `SELECT * FROM users WHERE id = $1 AND deleted_at IS NULL`
    
    err := r.db.GetContext(ctx, &user, query, id)
    if err != nil {
        if err == sql.ErrNoRows {
            return nil, ErrNotFound
        }
        return nil, err
    }
    
    return &user, nil
}

func (r *sqlxUserRepository) List(ctx context.Context, filter ListFilter) ([]*User, error) {
    users := []*User{}
    query := `
        SELECT * FROM users 
        WHERE deleted_at IS NULL 
        ORDER BY created_at DESC 
        LIMIT :limit OFFSET :offset
    `
    
    nstmt, err := r.db.PrepareNamedContext(ctx, query)
    if err != nil {
        return nil, err
    }
    
    err = nstmt.SelectContext(ctx, &users, filter)
    return users, err
}
```

## Using GORM

```go
// internal/repository/user_gorm.go
package repository

import "gorm.io/gorm"

type gormUserRepository struct {
    db *gorm.DB
}

func NewGORMUserRepository(db *gorm.DB) UserRepository {
    return &gormUserRepository{db: db}
}

func (r *gormUserRepository) Create(ctx context.Context, user *User) error {
    return r.db.WithContext(ctx).Create(user).Error
}

func (r *gormUserRepository) GetByID(ctx context.Context, id string) (*User, error) {
    var user User
    err := r.db.WithContext(ctx).Where("id = ?", id).First(&user).Error
    
    if err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            return nil, ErrNotFound
        }
        return nil, err
    }
    
    return &user, nil
}

func (r *gormUserRepository) List(ctx context.Context, filter ListFilter) ([]*User, error) {
    var users []*User
    
    query := r.db.WithContext(ctx).Model(&User{})
    
    if filter.Search != "" {
        query = query.Where("name LIKE ? OR email LIKE ?", 
            "%"+filter.Search+"%", 
            "%"+filter.Search+"%")
    }
    
    err := query.
        Limit(filter.Limit).
        Offset(filter.Offset).
        Order("created_at DESC").
        Find(&users).Error
        
    return users, err
}
```

## Database Migrations

```go
// internal/database/migrations.go
package database

import (
    "github.com/golang-migrate/migrate/v4"
    "github.com/golang-migrate/migrate/v4/database/postgres"
    _ "github.com/golang-migrate/migrate/v4/source/file"
)

func RunMigrations(db *sql.DB, migrationsPath string) error {
    driver, err := postgres.WithInstance(db, &postgres.Config{})
    if err != nil {
        return err
    }
    
    m, err := migrate.NewWithDatabaseInstance(
        migrationsPath,
        "postgres",
        driver,
    )
    if err != nil {
        return err
    }
    
    if err := m.Up(); err != nil && err != migrate.ErrNoChange {
        return err
    }
    
    return nil
}
```

## Query Builder Pattern

```go
// internal/repository/query_builder.go
package repository

type QueryBuilder struct {
    query  strings.Builder
    args   []interface{}
    argNum int
}

func NewQueryBuilder(base string) *QueryBuilder {
    qb := &QueryBuilder{argNum: 1}
    qb.query.WriteString(base)
    return qb
}

func (qb *QueryBuilder) Where(condition string, arg interface{}) *QueryBuilder {
    if qb.argNum == 1 {
        qb.query.WriteString(" WHERE ")
    } else {
        qb.query.WriteString(" AND ")
    }
    
    qb.query.WriteString(fmt.Sprintf(condition, fmt.Sprintf("$%d", qb.argNum)))
    qb.args = append(qb.args, arg)
    qb.argNum++
    
    return qb
}

func (qb *QueryBuilder) OrderBy(order string) *QueryBuilder {
    qb.query.WriteString(" ORDER BY ")
    qb.query.WriteString(order)
    return qb
}

func (qb *QueryBuilder) Limit(limit int) *QueryBuilder {
    qb.query.WriteString(fmt.Sprintf(" LIMIT $%d", qb.argNum))
    qb.args = append(qb.args, limit)
    qb.argNum++
    return qb
}

func (qb *QueryBuilder) Build() (string, []interface{}) {
    return qb.query.String(), qb.args
}
```

## Best Practices Summary

1. **Use connection pooling** with appropriate settings
2. **Always use prepared statements** to prevent SQL injection
3. **Handle database-specific errors** appropriately
4. **Use transactions** for multi-step operations
5. **Implement repository pattern** for testability
6. **Use context for timeouts** and cancellation
7. **Close rows after querying**
8. **Use database migrations** for schema management
9. **Monitor slow queries** and optimize
10. **Implement retry logic** for transient errors
11. **Keep adapters focused** on data translation
12. **Map between domain models and database models**
13. **Don't leak database details** into the core domain

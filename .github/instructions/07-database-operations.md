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

## Repository Pattern

```go
// internal/repository/interfaces.go
package repository

type UserRepository interface {
    Create(ctx context.Context, user *User) error
    GetByID(ctx context.Context, id string) (*User, error)
    GetByEmail(ctx context.Context, email string) (*User, error)
    Update(ctx context.Context, user *User) error
    Delete(ctx context.Context, id string) error
    List(ctx context.Context, filter ListFilter) ([]*User, error)
}

// internal/repository/user_postgres.go
package repository

import (
    "database/sql"
    "fmt"
)

type postgresUserRepository struct {
    db *sql.DB
}

func NewPostgresUserRepository(db *sql.DB) UserRepository {
    return &postgresUserRepository{db: db}
}

func (r *postgresUserRepository) Create(ctx context.Context, user *User) error {
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
            return ErrDuplicateEmail
        }
        return fmt.Errorf("failed to create user: %w", err)
    }
    
    return nil
}

func (r *postgresUserRepository) GetByID(ctx context.Context, id string) (*User, error) {
    query := `
        SELECT id, email, name, password_hash, created_at, updated_at
        FROM users
        WHERE id = $1 AND deleted_at IS NULL
    `
    
    var user User
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
            return nil, ErrNotFound
        }
        return nil, fmt.Errorf("failed to get user: %w", err)
    }
    
    return &user, nil
}

func (r *postgresUserRepository) List(ctx context.Context, filter ListFilter) ([]*User, error) {
    query := `
        SELECT id, email, name, created_at, updated_at
        FROM users
        WHERE deleted_at IS NULL
        ORDER BY created_at DESC
        LIMIT $1 OFFSET $2
    `
    
    rows, err := r.db.QueryContext(ctx, query, filter.Limit, filter.Offset)
    if err != nil {
        return nil, fmt.Errorf("failed to list users: %w", err)
    }
    defer rows.Close()
    
    var users []*User
    for rows.Next() {
        var user User
        if err := rows.Scan(
            &user.ID,
            &user.Email,
            &user.Name,
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

## Transaction Management

```go
// internal/repository/transaction.go
package repository

type TxRepository interface {
    UserRepository
    BeginTx(ctx context.Context) (TxRepository, error)
    Commit() error
    Rollback() error
}

type postgresTxRepository struct {
    tx *sql.Tx
    *postgresUserRepository
}

func (r *postgresUserRepository) BeginTx(ctx context.Context) (TxRepository, error) {
    tx, err := r.db.BeginTx(ctx, nil)
    if err != nil {
        return nil, err
    }
    
    return &postgresTxRepository{
        tx:                     tx,
        postgresUserRepository: &postgresUserRepository{db: nil},
    }, nil
}

// Transaction example in service
func (s *UserService) CreateUserWithProfile(ctx context.Context, input *CreateUserInput) error {
    tx, err := s.repo.BeginTx(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback()
    
    // Create user
    user := &User{
        ID:    uuid.New().String(),
        Email: input.Email,
        Name:  input.Name,
    }
    
    if err := tx.Create(ctx, user); err != nil {
        return err
    }
    
    // Create profile
    profile := &Profile{
        UserID: user.ID,
        Bio:    input.Bio,
    }
    
    if err := tx.CreateProfile(ctx, profile); err != nil {
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

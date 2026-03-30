package db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store wraps a pgxpool.Pool and provides all database operations.
// It is safe for concurrent use.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a Store from an existing pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Exec exposes pool.Exec for one-off queries from handlers.
func (s *Store) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return s.pool.Exec(ctx, sql, args...)
}

// QueryRow exposes pool.QueryRow for one-off queries from handlers.
func (s *Store) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return s.pool.QueryRow(ctx, sql, args...)
}

// Query exposes pool.Query for multi-row queries from handlers.
func (s *Store) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return s.pool.Query(ctx, sql, args...)
}

// Ping checks that the database connection is alive.
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

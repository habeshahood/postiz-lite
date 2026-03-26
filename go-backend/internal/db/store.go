package db

import "github.com/jackc/pgx/v5/pgxpool"

// Store wraps a pgxpool.Pool and provides all database operations.
// It is safe for concurrent use.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a Store from an existing pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

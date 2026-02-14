package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return errors.New("postgres pool is nil")
	}

	const createTasksTable = `
		CREATE TABLE IF NOT EXISTS tasks (
			id UUID PRIMARY KEY,
			method TEXT NOT NULL,
			url TEXT NOT NULL,
			status TEXT NOT NULL,
			request_headers JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ
		);
	`

	if _, err := pool.Exec(ctx, createTasksTable); err != nil {
		return fmt.Errorf("create tasks table: %w", err)
	}

	return nil
}

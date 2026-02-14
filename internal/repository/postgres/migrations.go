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
			http_status_code INTEGER NOT NULL DEFAULT 0,
			response_headers JSONB NOT NULL DEFAULT '{}'::jsonb,
			length BIGINT NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ
		);
	`

	if _, err := pool.Exec(ctx, createTasksTable); err != nil {
		return fmt.Errorf("create tasks table: %w", err)
	}

	statements := []string{
		`ALTER TABLE tasks ADD COLUMN IF NOT EXISTS http_status_code INTEGER NOT NULL DEFAULT 0;`,
		`ALTER TABLE tasks ADD COLUMN IF NOT EXISTS response_headers JSONB NOT NULL DEFAULT '{}'::jsonb;`,
		`ALTER TABLE tasks ADD COLUMN IF NOT EXISTS length BIGINT NOT NULL DEFAULT 0;`,
	}

	for _, sql := range statements {
		if _, err := pool.Exec(ctx, sql); err != nil {
			return fmt.Errorf("alter tasks table: %w", err)
		}
	}

	return nil
}

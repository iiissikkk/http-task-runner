package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNoAppliedMigrations = errors.New("no applied migrations")

func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return errors.New("postgres pool is nil")
	}

	if err := ensureMigrationsTable(ctx, pool); err != nil {
		return err
	}

	dir, err := migrationsDir()
	if err != nil {
		return err
	}

	paths, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		return fmt.Errorf("find migration files: %w", err)
	}
	if len(paths) == 0 {
		return fmt.Errorf("no up migrations found in %s", dir)
	}
	sort.Strings(paths)

	applied, err := loadAppliedMigrations(ctx, pool)
	if err != nil {
		return err
	}

	for _, path := range paths {
		version := filepath.Base(path)
		if _, ok := applied[version]; ok {
			continue
		}

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read migration %s: %w", version, readErr)
		}

		query := strings.TrimSpace(string(content))
		if execErr := execMigrationTx(ctx, pool, version, query); execErr != nil {
			return fmt.Errorf("apply migration %s: %w", version, execErr)
		}
	}

	return nil
}

func RollbackLastMigration(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return errors.New("postgres pool is nil")
	}

	if err := ensureMigrationsTable(ctx, pool); err != nil {
		return err
	}

	const query = `
		SELECT version
		FROM schema_migrations
		ORDER BY version DESC
		LIMIT 1
	`

	var version string
	if err := pool.QueryRow(ctx, query).Scan(&version); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoAppliedMigrations
		}
		return fmt.Errorf("select last applied migration: %w", err)
	}

	dir, err := migrationsDir()
	if err != nil {
		return err
	}

	downFile := strings.TrimSuffix(version, ".up.sql") + ".down.sql"
	downPath := filepath.Join(dir, downFile)

	content, err := os.ReadFile(downPath)
	if err != nil {
		return fmt.Errorf("read down migration %s: %w", downFile, err)
	}

	downQuery := strings.TrimSpace(string(content))
	if err := execRollbackTx(ctx, pool, version, downQuery); err != nil {
		return fmt.Errorf("rollback migration %s: %w", version, err)
	}

	return nil
}

func ensureMigrationsTable(ctx context.Context, pool *pgxpool.Pool) error {
	const query = `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`

	if _, err := pool.Exec(ctx, query); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	return nil
}

func loadAppliedMigrations(ctx context.Context, pool *pgxpool.Pool) (map[string]struct{}, error) {
	const query = `SELECT version FROM schema_migrations`

	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("select applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]struct{})
	for rows.Next() {
		var version string
		if scanErr := rows.Scan(&version); scanErr != nil {
			return nil, fmt.Errorf("scan applied migration: %w", scanErr)
		}
		applied[version] = struct{}{}
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applied migrations: %w", err)
	}

	return applied, nil
}

func migrationsDir() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("resolve migrations path: runtime caller failed")
	}

	dir := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "migrations"))
	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("resolve migrations path: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("resolve migrations path: %s is not a directory", dir)
	}

	return dir, nil
}

func execMigrationTx(ctx context.Context, pool *pgxpool.Pool, version string, query string) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	if query != "" {
		if _, err = tx.Exec(ctx, query); err != nil {
			return err
		}
	}

	if _, err = tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
		return fmt.Errorf("insert applied migration: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	committed = true
	return nil
}

func execRollbackTx(ctx context.Context, pool *pgxpool.Pool, version string, query string) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	if query != "" {
		if _, err = tx.Exec(ctx, query); err != nil {
			return err
		}
	}

	if _, err = tx.Exec(ctx, `DELETE FROM schema_migrations WHERE version = $1`, version); err != nil {
		return fmt.Errorf("delete applied migration: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	committed = true
	return nil
}

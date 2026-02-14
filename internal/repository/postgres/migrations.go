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

	"github.com/jackc/pgx/v5/pgxpool"
)

func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return errors.New("postgres pool is nil")
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

	for _, path := range paths {
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read migration %s: %w", filepath.Base(path), readErr)
		}

		query := strings.TrimSpace(string(content))
		if query == "" {
			continue
		}

		if _, execErr := pool.Exec(ctx, query); execErr != nil {
			return fmt.Errorf("apply migration %s: %w", filepath.Base(path), execErr)
		}
	}

	return nil
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

package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"todoapp/internal/config"
	"todoapp/internal/repository/postgres"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Println("failed to load config:", err)
		os.Exit(1)
	}

	pool, err := postgres.NewPool(context.Background(), cfg.DatabaseURL)
	if err != nil {
		fmt.Println("failed to connect to postgres:", err)
		os.Exit(1)
	}
	defer pool.Close()

	mode := "up"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

	switch mode {
	case "up":
		if err = postgres.RunMigrations(context.Background(), pool); err != nil {
			fmt.Println("failed to run migrations:", err)
			os.Exit(1)
		}
		fmt.Println("migrations applied")
	case "down":
		if err = postgres.RollbackLastMigration(context.Background(), pool); err != nil {
			if errors.Is(err, postgres.ErrNoAppliedMigrations) {
				fmt.Println("no applied migrations to rollback")
				return
			}
			fmt.Println("failed to rollback migration:", err)
			os.Exit(1)
		}
		fmt.Println("last migration rolled back")
	default:
		fmt.Println("unknown mode, use: up or down")
		os.Exit(1)
	}
}

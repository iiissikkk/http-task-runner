package main

import (
	"context"
	"errors"
	"os"

	"todoapp/internal/config"
	"todoapp/internal/repository/postgres"

	"github.com/sirupsen/logrus"
)

func main() {
	logger := logrus.New()

	cfg, err := config.Load()
	if err != nil {
		logger.WithError(err).Error("failed to load config")
		os.Exit(1)
	}

	logger.SetLevel(cfg.LogLevel)
	switch cfg.LogFormat {
	case "json":
		logger.SetFormatter(&logrus.JSONFormatter{})
	default:
		logger.SetFormatter(&logrus.TextFormatter{})
	}

	pool, err := postgres.NewPool(context.Background(), cfg.DatabaseURL)
	if err != nil {
		logger.WithError(err).Error("failed to connect to postgres")
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
			logger.WithError(err).Error("failed to run migrations")
			os.Exit(1)
		}
		logger.Info("migrations applied")
	case "down":
		if err = postgres.RollbackLastMigration(context.Background(), pool); err != nil {
			if errors.Is(err, postgres.ErrNoAppliedMigrations) {
				logger.Info("no applied migrations to rollback")
				return
			}
			logger.WithError(err).Error("failed to rollback migration")
			os.Exit(1)
		}
		logger.Info("last migration rolled back")
	default:
		logger.Error("unknown mode, use: up or down")
		os.Exit(1)
	}
}

package main

import (
	"context"
	"errors"
	stdhttp "net/http"
	"os"
	"os/signal"
	"syscall"

	"todoapp/internal/adapter/executor"
	"todoapp/internal/config"
	delivery "todoapp/internal/delivery/http"
	"todoapp/internal/repository/postgres"
	service "todoapp/internal/usecase/task"

	"github.com/sirupsen/logrus"
)

func main() {
	logger := logrus.New()

	cfg, err := config.Load()
	if err != nil {
		logger.WithError(err).Error("failed to load config")
		return
	}

	logger.SetLevel(cfg.LogLevel)
	switch cfg.LogFormat {
	case "json":
		logger.SetFormatter(&logrus.JSONFormatter{})
	default:
		logger.SetFormatter(&logrus.TextFormatter{})
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, sqlDB, err := postgres.NewPool(context.Background(), cfg.DatabaseURL)
	if err != nil {
		logger.WithError(err).Error("failed to connect to postgres")
		return
	}
	defer sqlDB.Close()

	if err = postgres.RunMigrations(context.Background(), db); err != nil {
		logger.WithError(err).Error("failed to run migrations on startup")
		return
	}

	store := postgres.NewStore(db)
	httpExecutor := executor.NewHTTPExecutor(cfg.HTTPExecutorTimeout)
	taskService := service.NewService(store, httpExecutor, store)
	apiHandler := delivery.NewHandler(taskService, store, cfg.HTTPPort, logger)
	router := delivery.NewRouter(apiHandler)

	server := delivery.NewServer(delivery.ServerConfig{
		Addr:         cfg.HTTPAddr,
		Handler:      router,
		ReadTimeout:  cfg.HTTPReadTimeout,
		WriteTimeout: cfg.HTTPWriteTimeout,
		IdleTimeout:  cfg.HTTPIdleTimeout,
	})

	errCh := make(chan error, 1)
	logger.WithField("addr", cfg.HTTPAddr).Info("starting http server")
	go func() {
		errCh <- server.Start()
	}()

	select {
	case err = <-errCh:
		if err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
			logger.WithError(err).Error("failed to start http server")
		}
		return
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTPShutdownTimeout)
	defer cancel()

	if err = server.Shutdown(shutdownCtx); err != nil {
		logger.WithError(err).Error("failed to shutdown http server gracefully")
		return
	}

	if err = <-errCh; err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
		logger.WithError(err).Error("http server stopped with error")
	}
}

package main

import (
	"context"
	"errors"
	"fmt"
	stdhttp "net/http"
	"os"
	"os/signal"
	"syscall"

	"todoapp/internal/adapter/executor"
	"todoapp/internal/config"
	delivery "todoapp/internal/delivery/http"
	"todoapp/internal/repository/postgres"
	"todoapp/internal/usecase/task"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Println("failed to load config:", err)
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.NewPool(context.Background(), cfg.DatabaseURL)
	if err != nil {
		fmt.Println("failed to connect to postgres:", err)
		return
	}
	defer pool.Close()

	if err = postgres.RunMigrations(context.Background(), pool); err != nil {
		fmt.Println("failed to run postgres migrations:", err)
		return
	}

	store := postgres.NewStore(pool)
	httpExecutor := executor.NewHTTPExecutor(cfg.HTTPExecutorTimeout)
	taskService := service.NewService(store, httpExecutor)
	handlers := delivery.NewHandlers(taskService, store, cfg.HTTPPort)
	router := delivery.NewRouter(handlers)

	server := delivery.NewServer(delivery.ServerConfig{
		Addr:         cfg.HTTPAddr,
		Handler:      router,
		ReadTimeout:  cfg.HTTPReadTimeout,
		WriteTimeout: cfg.HTTPWriteTimeout,
		IdleTimeout:  cfg.HTTPIdleTimeout,
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start()
	}()

	select {
	case err = <-errCh:
		if err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
			fmt.Println("failed to start http server:", err)
		}
		return
	case <-ctx.Done():
		fmt.Println("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTPShutdownTimeout)
	defer cancel()

	if err = server.Shutdown(shutdownCtx); err != nil {
		fmt.Println("failed to shutdown http server gracefully:", err)
		return
	}

	if err = <-errCh; err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
		fmt.Println("http server stopped with error:", err)
	}
}

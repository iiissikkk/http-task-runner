package main

import (
	"fmt"
	"time"

	"todoapp/internal/adapter/executor"
	"todoapp/internal/delivery/http"
	"todoapp/internal/repository/task"
	"todoapp/internal/usecase/task"
)

func main() {
	addr := ":9091"
	port := "9091"

	store := store.NewMemoryStore()
	httpExecutor := executor.NewHTTPExecutor(10 * time.Second)
	service := service.NewService(store, httpExecutor)
	handlers := delivery.NewHandlers(service, port)
	router := delivery.NewRouter(handlers)
	server := delivery.NewServer(addr, router)

	if err := server.Start(); err != nil {
		fmt.Println("failed to start http server:", err)
	}
}

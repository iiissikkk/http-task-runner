package main

import (
	"fmt"
	"todoapp/http"
	"todoapp/todo"
)

func main() {
	todoList := todo.NewList()
	httpHandlers := http.NewHTTPHandlers(todoList)
	httpServer := http.NewHTTPServer(httpHandlers)

	if err := httpServer.StartServer(); err != nil {
		fmt.Println("failed to start http server:", err)
	}
}

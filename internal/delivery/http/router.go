package delivery

import (
	"net/http"

	"github.com/gorilla/mux"
)

func NewRouter(handlers *Handlers) http.Handler {
	router := mux.NewRouter()
	router.HandleFunc("/task", handlers.CreateTask).Methods(http.MethodPost)
	router.HandleFunc("/tasks", handlers.GetAllTasks).Methods(http.MethodGet)
	router.HandleFunc("/task/{id}", handlers.GetTask).Methods(http.MethodGet)
	router.HandleFunc("/task/{id}", handlers.DeleteTask).Methods(http.MethodDelete)
	router.HandleFunc("/healthz", handlers.Healthz).Methods(http.MethodGet)
	return router
}

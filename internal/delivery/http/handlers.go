package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"todoapp/internal/domain/task"
	"todoapp/internal/usecase/task"

	"github.com/gorilla/mux"
)

type TaskService interface {
	CreateTask(ctx context.Context, input service.CreateInput) (string, error)
	GetTask(ctx context.Context, id string) (task.Task, error)
	GetAllTasks(ctx context.Context) ([]task.Task, error)
	DeleteTask(ctx context.Context, id string) (task.Task, error)
}

type HealthChecker interface {
	Ping(ctx context.Context) error
}

type Handlers struct {
	service       TaskService
	healthChecker HealthChecker
	port          string
}

func NewHandlers(taskService TaskService, healthChecker HealthChecker, port string) *Handlers {
	return &Handlers{
		service:       taskService,
		healthChecker: healthChecker,
		port:          port,
	}
}

func (h *Handlers) CreateTask(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		defer r.Body.Close()
	}

	var req createTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}

	id, err := h.service.CreateTask(r.Context(), service.CreateInput{
		Method:  req.Method,
		URL:     req.URL,
		Headers: req.Headers,
	})
	if err != nil {
		writeJSON(w, mapDomainErrorToStatus(err), errorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, createTaskResponse{ID: id})
}

func (h *Handlers) GetTask(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if id == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid task id in path"})
		return
	}

	item, err := h.service.GetTask(r.Context(), id)
	if err != nil {
		writeJSON(w, mapDomainErrorToStatus(err), errorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, getTaskResponse{
		ID:             item.ID,
		Status:         string(item.Status),
		HTTPStatusCode: item.HTTPStatusCode,
		Headers:        item.Headers,
		Length:         item.Length,
	})
}

func (h *Handlers) GetAllTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := h.service.GetAllTasks(r.Context())
	if err != nil {
		writeJSON(w, mapDomainErrorToStatus(err), errorResponse{Error: err.Error()})
		return
	}

	resp := make([]getTaskResponse, 0, len(tasks))
	for _, t := range tasks {
		resp = append(resp, getTaskResponse{
			ID:             t.ID,
			Status:         string(t.Status),
			HTTPStatusCode: t.HTTPStatusCode,
			Headers:        t.Headers,
			Length:         t.Length,
		})
	}

	writeJSON(w, http.StatusOK, getAllTasksResponse{Tasks: resp})
}

func (h *Handlers) DeleteTask(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if id == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid task id in path"})
		return
	}

	item, err := h.service.DeleteTask(r.Context(), id)
	if err != nil {
		writeJSON(w, mapDomainErrorToStatus(err), errorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, deleteTaskResponse{
		ID:             item.ID,
		HTTPStatusCode: item.HTTPStatusCode,
	})
}

func (h *Handlers) Healthz(w http.ResponseWriter, r *http.Request) {
	if h.healthChecker != nil {
		if err := h.healthChecker.Ping(r.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, getHealthStatus{
				Status: "unavailable",
				Port:   h.port,
			})
			return
		}
	}

	writeJSON(w, http.StatusOK, getHealthStatus{
		Status: "ok",
		Port:   h.port,
	})
}

func mapDomainErrorToStatus(err error) int {
	switch {
	case errors.Is(err, task.ErrTaskNotFound):
		return http.StatusNotFound
	case errors.Is(err, task.ErrInvalidMethod),
		errors.Is(err, task.ErrInvalidURL),
		errors.Is(err, task.ErrMethodRequired),
		errors.Is(err, task.ErrURLRequired):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

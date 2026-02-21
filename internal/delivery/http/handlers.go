package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"todoapp/internal/domain/task"
	service "todoapp/internal/usecase/task"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
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
	logger        *logrus.Logger
}

func NewHandlers(taskService TaskService, healthChecker HealthChecker, port string, logger *logrus.Logger) *Handlers {
	if logger == nil {
		logger = logrus.New()
	}

	return &Handlers{
		service:       taskService,
		healthChecker: healthChecker,
		port:          port,
		logger:        logger,
	}
}

func (h *Handlers) CreateTask(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		defer r.Body.Close()
	}

	var req createTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		status := http.StatusBadRequest
		h.logError(r, "", status, "invalid create task request body", err)
		writeJSON(w, status, errorResponse{Error: err.Error()})
		return
	}

	h.logEntry(r, nil).Info("create task request received")

	id, err := h.service.CreateTask(r.Context(), service.CreateInput{
		Method:  req.Method,
		URL:     req.URL,
		Headers: req.Headers,
	})
	if err != nil {
		status := mapDomainErrorToStatus(err)
		h.logError(r, "", status, "failed to create task", err)
		writeJSON(w, status, errorResponse{Error: err.Error()})
		return
	}

	h.logEntry(r, logrus.Fields{
		"task_id": id,
		"status":  http.StatusOK,
	}).Info("task created")

	writeJSON(w, http.StatusOK, createTaskResponse{ID: id})
}

func (h *Handlers) GetTask(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if id == "" {
		status := http.StatusBadRequest
		h.logError(r, "", status, "invalid task id in path", nil)
		writeJSON(w, status, errorResponse{Error: "invalid task id in path"})
		return
	}

	h.logEntry(r, nil).Info("get task request received")

	item, err := h.service.GetTask(r.Context(), id)
	if err != nil {
		status := mapDomainErrorToStatus(err)
		h.logError(r, id, status, "failed to get task", err)
		writeJSON(w, status, errorResponse{Error: err.Error()})
		return
	}

	h.logEntry(r, logrus.Fields{
		"task_id": id,
		"status":  http.StatusOK,
	}).Info("get task")

	writeJSON(w, http.StatusOK, getTaskResponse{
		ID:             item.ID,
		Status:         string(item.Status),
		HTTPStatusCode: item.HTTPStatusCode,
		Headers:        item.Headers,
		Length:         item.Length,
	})
}

func (h *Handlers) GetAllTasks(w http.ResponseWriter, r *http.Request) {
	h.logEntry(r, nil).Info("get all tasks request received")

	tasks, err := h.service.GetAllTasks(r.Context())
	if err != nil {
		status := mapDomainErrorToStatus(err)
		h.logError(r, "", status, "failed to list tasks", err)
		writeJSON(w, status, errorResponse{Error: err.Error()})
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

	h.logEntry(r, logrus.Fields{
		"count":  len(tasks),
		"status": http.StatusOK,
	}).Info("tasks listed")

	writeJSON(w, http.StatusOK, getAllTasksResponse{Tasks: resp})
}

func (h *Handlers) DeleteTask(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if id == "" {
		status := http.StatusBadRequest
		h.logError(r, "", status, "invalid task id in path", nil)
		writeJSON(w, status, errorResponse{Error: "invalid task id in path"})
		return
	}

	h.logEntry(r, nil).Info("delete task request received")

	item, err := h.service.DeleteTask(r.Context(), id)
	if err != nil {
		status := mapDomainErrorToStatus(err)
		h.logError(r, id, status, "failed to delete task", err)
		writeJSON(w, status, errorResponse{Error: err.Error()})
		return
	}

	h.logEntry(r, logrus.Fields{
		"task_id": id,
		"status":  http.StatusOK,
	}).Info("delete task")

	writeJSON(w, http.StatusOK, deleteTaskResponse{
		ID:             item.ID,
		HTTPStatusCode: item.HTTPStatusCode,
	})
}

func (h *Handlers) Healthz(w http.ResponseWriter, r *http.Request) {
	if h.healthChecker == nil {
		status := http.StatusServiceUnavailable
		h.logError(r, "", status, "health checker is unavailable", nil)
		writeJSON(w, status, getHealthStatus{
			Status: "unavailable",
			Port:   h.port,
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.healthChecker.Ping(ctx); err != nil {
		status := http.StatusServiceUnavailable
		h.logError(r, "", status, "health checker ping failed", err)
		writeJSON(w, status, getHealthStatus{
			Status: "unavailable",
			Port:   h.port,
		})
		return
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

func (h *Handlers) logEntry(r *http.Request, extra logrus.Fields) *logrus.Entry {
	fields := logrus.Fields{
		"endpoint":    r.Method + " " + r.URL.Path,
		"http_method": r.Method,
		"path":        r.URL.Path,
	}
	for k, v := range extra {
		fields[k] = v
	}
	return h.logger.WithFields(fields)
}

func (h *Handlers) logError(r *http.Request, taskID string, status int, message string, err error) {
	fields := logrus.Fields{
		"status": status,
	}
	if taskID != "" {
		fields["task_id"] = taskID
	}

	entry := h.logEntry(r, fields)
	if err != nil {
		entry = entry.WithError(err)
	}
	entry.Error(message)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

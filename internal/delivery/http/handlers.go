package delivery

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gorilla/mux"
	"todoapp/internal/domain/task"
	"todoapp/internal/usecase/task"
)

type Handlers struct {
	service *service.Service
}

func NewHandlers(service *service.Service) *Handlers {
	return &Handlers{service: service}
}

func (h *Handlers) CreateTask(w http.ResponseWriter, r *http.Request) {
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

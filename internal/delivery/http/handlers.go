package delivery

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	httpopenapi "todoapp/internal/delivery/http/openapi"
	"todoapp/internal/domain/task"
	service "todoapp/internal/usecase/task"

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

type Handler struct {
	service       TaskService
	healthChecker HealthChecker
	port          string
	logger        *logrus.Logger
}

var _ httpopenapi.StrictServerInterface = (*Handler)(nil)

func NewHandler(taskService TaskService, healthChecker HealthChecker, port string, logger *logrus.Logger) *Handler {
	if logger == nil {
		logger = logrus.New()
	}

	return &Handler{
		service:       taskService,
		healthChecker: healthChecker,
		port:          port,
		logger:        logger,
	}
}

func (s *Handler) CreateTask(ctx context.Context, request httpopenapi.CreateTaskRequestObject) (httpopenapi.CreateTaskResponseObject, error) {
	r := makeRequest(ctx, http.MethodPost, "/task")

	if request.Body == nil {
		status := http.StatusBadRequest
		s.logError(r, "", status, "invalid create task request body", nil)
		return createTaskErrorResponse(status, "request body is required"), nil
	}

	s.logEntry(r, nil).Info("create task request received")

	var headers map[string]string
	if request.Body.Headers != nil {
		headers = *request.Body.Headers
	}

	id, err := s.service.CreateTask(ctx, service.CreateInput{
		Method:  request.Body.Method,
		URL:     request.Body.Url,
		Headers: headers,
	})
	if err != nil {
		status := mapDomainErrorToStatus(err)
		s.logError(r, "", status, "failed to create task", err)
		return createTaskErrorResponse(status, err.Error()), nil
	}

	s.logEntry(r, logrus.Fields{
		"task_id": id,
		"status":  http.StatusOK,
	}).Info("task created")

	return httpopenapi.CreateTask200JSONResponse(httpopenapi.CreateTaskResponse{
		Id: stringPtr(id),
	}), nil
}

func (s *Handler) GetTask(ctx context.Context, request httpopenapi.GetTaskRequestObject) (httpopenapi.GetTaskResponseObject, error) {
	r := makeRequest(ctx, http.MethodGet, "/task/"+request.Id)
	if request.Id == "" {
		status := http.StatusBadRequest
		s.logError(r, "", status, "invalid task id in path", nil)
		return getTaskErrorResponse(status, "invalid task id in path"), nil
	}

	s.logEntry(r, nil).Info("get task request received")

	item, err := s.service.GetTask(ctx, request.Id)
	if err != nil {
		status := mapDomainErrorToStatus(err)
		s.logError(r, request.Id, status, "failed to get task", err)
		return getTaskErrorResponse(status, err.Error()), nil
	}

	s.logEntry(r, logrus.Fields{
		"task_id": request.Id,
		"status":  http.StatusOK,
	}).Info("get task")

	return httpopenapi.GetTask200JSONResponse(toOpenAPIGetTaskResponse(item)), nil
}

func (s *Handler) GetAllTasks(ctx context.Context, _ httpopenapi.GetAllTasksRequestObject) (httpopenapi.GetAllTasksResponseObject, error) {
	r := makeRequest(ctx, http.MethodGet, "/tasks")
	s.logEntry(r, nil).Info("get all tasks request received")

	tasks, err := s.service.GetAllTasks(ctx)
	if err != nil {
		status := mapDomainErrorToStatus(err)
		s.logError(r, "", status, "failed to list tasks", err)
		return getAllTasksErrorResponse(status, err.Error()), nil
	}

	items := make([]httpopenapi.GetTaskResponse, 0, len(tasks))
	for _, t := range tasks {
		items = append(items, toOpenAPIGetTaskResponse(t))
	}

	s.logEntry(r, logrus.Fields{
		"count":  len(tasks),
		"status": http.StatusOK,
	}).Info("tasks listed")

	return httpopenapi.GetAllTasks200JSONResponse(httpopenapi.GetAllTasksResponse{
		Tasks: &items,
	}), nil
}

func (s *Handler) DeleteTask(ctx context.Context, request httpopenapi.DeleteTaskRequestObject) (httpopenapi.DeleteTaskResponseObject, error) {
	r := makeRequest(ctx, http.MethodDelete, "/task/"+request.Id)
	if request.Id == "" {
		status := http.StatusBadRequest
		s.logError(r, "", status, "invalid task id in path", nil)
		return deleteTaskErrorResponse(status, "invalid task id in path"), nil
	}

	s.logEntry(r, nil).Info("delete task request received")

	item, err := s.service.DeleteTask(ctx, request.Id)
	if err != nil {
		status := mapDomainErrorToStatus(err)
		s.logError(r, request.Id, status, "failed to delete task", err)
		return deleteTaskErrorResponse(status, err.Error()), nil
	}

	s.logEntry(r, logrus.Fields{
		"task_id": request.Id,
		"status":  http.StatusOK,
	}).Info("delete task")

	return httpopenapi.DeleteTask200JSONResponse(httpopenapi.DeleteTaskResponse{
		Id:             stringPtr(item.ID),
		HttpStatusCode: intPtr(item.HTTPStatusCode),
	}), nil
}

func (s *Handler) Healthz(ctx context.Context, _ httpopenapi.HealthzRequestObject) (httpopenapi.HealthzResponseObject, error) {
	r := makeRequest(ctx, http.MethodGet, "/healthz")
	if s.healthChecker == nil {
		status := http.StatusServiceUnavailable
		s.logError(r, "", status, "health checker is unavailable", nil)
		return httpopenapi.Healthz503JSONResponse(httpopenapi.HealthResponse{
			Status: healthStatusPtr(httpopenapi.Unavailable),
			Port:   stringPtr(s.port),
		}), nil
	}

	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := s.healthChecker.Ping(pingCtx); err != nil {
		status := http.StatusServiceUnavailable
		s.logError(r, "", status, "health checker ping failed", err)
		return httpopenapi.Healthz503JSONResponse(httpopenapi.HealthResponse{
			Status: healthStatusPtr(httpopenapi.Unavailable),
			Port:   stringPtr(s.port),
		}), nil
	}

	return httpopenapi.Healthz200JSONResponse(httpopenapi.HealthResponse{
		Status: healthStatusPtr(httpopenapi.Ok),
		Port:   stringPtr(s.port),
	}), nil
}

func makeRequest(ctx context.Context, method, path string) *http.Request {
	r := &http.Request{
		Method: method,
		URL:    &url.URL{Path: path},
	}
	if ctx != nil {
		return r.WithContext(ctx)
	}
	return r
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

func (s *Handler) logEntry(r *http.Request, extra logrus.Fields) *logrus.Entry {
	fields := logrus.Fields{
		"endpoint":    r.Method + " " + r.URL.Path,
		"http_method": r.Method,
		"path":        r.URL.Path,
	}
	for k, v := range extra {
		fields[k] = v
	}
	return s.logger.WithFields(fields)
}

func (s *Handler) logError(r *http.Request, taskID string, status int, message string, err error) {
	fields := logrus.Fields{
		"status": status,
	}
	if taskID != "" {
		fields["task_id"] = taskID
	}

	entry := s.logEntry(r, fields)
	if err != nil {
		entry = entry.WithError(err)
	}
	entry.Error(message)
}

func toOpenAPIGetTaskResponse(t task.Task) httpopenapi.GetTaskResponse {
	status := httpopenapi.GetTaskResponseStatus(t.Status)
	return httpopenapi.GetTaskResponse{
		Id:             stringPtr(t.ID),
		Status:         &status,
		HttpStatusCode: intPtr(t.HTTPStatusCode),
		Headers:        mapStringSlicePtr(t.Headers),
		Length:         int64Ptr(t.Length),
	}
}

func createTaskErrorResponse(status int, message string) httpopenapi.CreateTaskResponseObject {
	errBody := httpopenapi.ErrorResponse{Error: stringPtr(message)}
	switch status {
	case http.StatusBadRequest:
		return httpopenapi.CreateTask400JSONResponse(errBody)
	default:
		return httpopenapi.CreateTask500JSONResponse(errBody)
	}
}

func getTaskErrorResponse(status int, message string) httpopenapi.GetTaskResponseObject {
	errBody := httpopenapi.ErrorResponse{Error: stringPtr(message)}
	switch status {
	case http.StatusBadRequest:
		return httpopenapi.GetTask400JSONResponse(errBody)
	case http.StatusNotFound:
		return httpopenapi.GetTask404JSONResponse(errBody)
	default:
		return httpopenapi.GetTask500JSONResponse(errBody)
	}
}

func getAllTasksErrorResponse(status int, message string) httpopenapi.GetAllTasksResponseObject {
	errBody := httpopenapi.ErrorResponse{Error: stringPtr(message)}
	switch status {
	default:
		_ = status
		return httpopenapi.GetAllTasks500JSONResponse(errBody)
	}
}

func deleteTaskErrorResponse(status int, message string) httpopenapi.DeleteTaskResponseObject {
	errBody := httpopenapi.ErrorResponse{Error: stringPtr(message)}
	switch status {
	case http.StatusBadRequest:
		return httpopenapi.DeleteTask400JSONResponse(errBody)
	case http.StatusNotFound:
		return httpopenapi.DeleteTask404JSONResponse(errBody)
	default:
		return httpopenapi.DeleteTask500JSONResponse(errBody)
	}
}

func stringPtr(v string) *string {
	return &v
}

func intPtr(v int) *int {
	return &v
}

func int64Ptr(v int64) *int64 {
	return &v
}

func mapStringSlicePtr(v map[string][]string) *map[string][]string {
	if v == nil {
		return nil
	}
	return &v
}

func healthStatusPtr(v httpopenapi.HealthResponseStatus) *httpopenapi.HealthResponseStatus {
	return &v
}

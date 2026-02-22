package delivery

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"todoapp/internal/domain/task"
	httpopenapi "todoapp/internal/delivery/http/openapi"
	service "todoapp/internal/usecase/task"

	"github.com/sirupsen/logrus"
)

type openAPIStrictServer struct {
	handlers *Handlers
}

var _ httpopenapi.StrictServerInterface = (*openAPIStrictServer)(nil)

func newOpenAPIStrictServer(handlers *Handlers) *openAPIStrictServer {
	return &openAPIStrictServer{handlers: handlers}
}

func (s *openAPIStrictServer) CreateTask(ctx context.Context, request httpopenapi.CreateTaskRequestObject) (httpopenapi.CreateTaskResponseObject, error) {
	r := makeRequest(ctx, http.MethodPost, "/task")

	if request.Body == nil {
		status := http.StatusBadRequest
		s.handlers.logError(r, "", status, "invalid create task request body", nil)
		return createTaskErrorResponse(status, "request body is required"), nil
	}

	s.handlers.logEntry(r, nil).Info("create task request received")

	var headers map[string]string
	if request.Body.Headers != nil {
		headers = *request.Body.Headers
	}

	id, err := s.handlers.service.CreateTask(ctx, service.CreateInput{
		Method:  request.Body.Method,
		URL:     request.Body.Url,
		Headers: headers,
	})
	if err != nil {
		status := mapDomainErrorToStatus(err)
		s.handlers.logError(r, "", status, "failed to create task", err)
		return createTaskErrorResponse(status, err.Error()), nil
	}

	s.handlers.logEntry(r, logrus.Fields{
		"task_id": id,
		"status":  http.StatusOK,
	}).Info("task created")

	return httpopenapi.CreateTask200JSONResponse(httpopenapi.CreateTaskResponse{
		Id: stringPtr(id),
	}), nil
}

func (s *openAPIStrictServer) GetTask(ctx context.Context, request httpopenapi.GetTaskRequestObject) (httpopenapi.GetTaskResponseObject, error) {
	r := makeRequest(ctx, http.MethodGet, "/task/"+request.Id)
	if request.Id == "" {
		status := http.StatusBadRequest
		s.handlers.logError(r, "", status, "invalid task id in path", nil)
		return getTaskErrorResponse(status, "invalid task id in path"), nil
	}

	s.handlers.logEntry(r, nil).Info("get task request received")

	item, err := s.handlers.service.GetTask(ctx, request.Id)
	if err != nil {
		status := mapDomainErrorToStatus(err)
		s.handlers.logError(r, request.Id, status, "failed to get task", err)
		return getTaskErrorResponse(status, err.Error()), nil
	}

	s.handlers.logEntry(r, logrus.Fields{
		"task_id": request.Id,
		"status":  http.StatusOK,
	}).Info("get task")

	return httpopenapi.GetTask200JSONResponse(toOpenAPIGetTaskResponse(item)), nil
}

func (s *openAPIStrictServer) GetAllTasks(ctx context.Context, _ httpopenapi.GetAllTasksRequestObject) (httpopenapi.GetAllTasksResponseObject, error) {
	r := makeRequest(ctx, http.MethodGet, "/tasks")
	s.handlers.logEntry(r, nil).Info("get all tasks request received")

	tasks, err := s.handlers.service.GetAllTasks(ctx)
	if err != nil {
		status := mapDomainErrorToStatus(err)
		s.handlers.logError(r, "", status, "failed to list tasks", err)
		return getAllTasksErrorResponse(status, err.Error()), nil
	}

	items := make([]httpopenapi.GetTaskResponse, 0, len(tasks))
	for _, t := range tasks {
		items = append(items, toOpenAPIGetTaskResponse(t))
	}

	s.handlers.logEntry(r, logrus.Fields{
		"count":  len(tasks),
		"status": http.StatusOK,
	}).Info("tasks listed")

	return httpopenapi.GetAllTasks200JSONResponse(httpopenapi.GetAllTasksResponse{
		Tasks: &items,
	}), nil
}

func (s *openAPIStrictServer) DeleteTask(ctx context.Context, request httpopenapi.DeleteTaskRequestObject) (httpopenapi.DeleteTaskResponseObject, error) {
	r := makeRequest(ctx, http.MethodDelete, "/task/"+request.Id)
	if request.Id == "" {
		status := http.StatusBadRequest
		s.handlers.logError(r, "", status, "invalid task id in path", nil)
		return deleteTaskErrorResponse(status, "invalid task id in path"), nil
	}

	s.handlers.logEntry(r, nil).Info("delete task request received")

	item, err := s.handlers.service.DeleteTask(ctx, request.Id)
	if err != nil {
		status := mapDomainErrorToStatus(err)
		s.handlers.logError(r, request.Id, status, "failed to delete task", err)
		return deleteTaskErrorResponse(status, err.Error()), nil
	}

	s.handlers.logEntry(r, logrus.Fields{
		"task_id": request.Id,
		"status":  http.StatusOK,
	}).Info("delete task")

	return httpopenapi.DeleteTask200JSONResponse(httpopenapi.DeleteTaskResponse{
		Id:             stringPtr(item.ID),
		HttpStatusCode: intPtr(item.HTTPStatusCode),
	}), nil
}

func (s *openAPIStrictServer) Healthz(ctx context.Context, _ httpopenapi.HealthzRequestObject) (httpopenapi.HealthzResponseObject, error) {
	r := makeRequest(ctx, http.MethodGet, "/healthz")
	if s.handlers.healthChecker == nil {
		status := http.StatusServiceUnavailable
		s.handlers.logError(r, "", status, "health checker is unavailable", nil)
		return httpopenapi.Healthz503JSONResponse(httpopenapi.HealthResponse{
			Status: healthStatusPtr(httpopenapi.Unavailable),
			Port:   stringPtr(s.handlers.port),
		}), nil
	}

	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := s.handlers.healthChecker.Ping(pingCtx); err != nil {
		status := http.StatusServiceUnavailable
		s.handlers.logError(r, "", status, "health checker ping failed", err)
		return httpopenapi.Healthz503JSONResponse(httpopenapi.HealthResponse{
			Status: healthStatusPtr(httpopenapi.Unavailable),
			Port:   stringPtr(s.handlers.port),
		}), nil
	}

	return httpopenapi.Healthz200JSONResponse(httpopenapi.HealthResponse{
		Status: healthStatusPtr(httpopenapi.Ok),
		Port:   stringPtr(s.handlers.port),
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

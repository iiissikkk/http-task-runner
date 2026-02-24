package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"todoapp/internal/domain/task"
	service "todoapp/internal/usecase/task"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

type mockTaskService struct {
	createTaskFn  func(ctx context.Context, input service.CreateInput) (string, error)
	getTaskFn     func(ctx context.Context, id string) (task.Task, error)
	getAllTasksFn func(ctx context.Context) ([]task.Task, error)
	deleteTaskFn  func(ctx context.Context, id string) (task.Task, error)
}

func (m *mockTaskService) CreateTask(ctx context.Context, input service.CreateInput) (string, error) {
	if m.createTaskFn == nil {
		return "", nil
	}
	return m.createTaskFn(ctx, input)
}

func (m *mockTaskService) GetTask(ctx context.Context, id string) (task.Task, error) {
	if m.getTaskFn == nil {
		return task.Task{}, nil
	}
	return m.getTaskFn(ctx, id)
}

func (m *mockTaskService) GetAllTasks(ctx context.Context) ([]task.Task, error) {
	if m.getAllTasksFn == nil {
		return nil, nil
	}
	return m.getAllTasksFn(ctx)
}

func (m *mockTaskService) DeleteTask(ctx context.Context, id string) (task.Task, error) {
	if m.deleteTaskFn == nil {
		return task.Task{}, nil
	}
	return m.deleteTaskFn(ctx, id)
}

type mockHealthChecker struct {
	pingErr error
}

func (m mockHealthChecker) Ping(_ context.Context) error {
	return m.pingErr
}

func newTestLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	return logger
}

func TestHandlersCreateTask(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		body            string
		createTaskFn    func(ctx context.Context, input service.CreateInput) (string, error)
		wantStatus      int
		wantContentType string
		wantJSONBody    bool
		wantID          string
		wantErrorSubstr string
	}{
		{
			name: "success",
			body: `{"method":"GET","url":"https://example.com","headers":{"Authorization":"token"}}`,
			createTaskFn: func(_ context.Context, input service.CreateInput) (string, error) {
				if input.Method != "GET" {
					return "", errors.New("wrong method passed to service")
				}
				if input.URL != "https://example.com" {
					return "", errors.New("wrong url passed to service")
				}
				return "task-1", nil
			},
			wantStatus:      http.StatusOK,
			wantContentType: "application/json",
			wantJSONBody:    true,
			wantID:          "task-1",
		},
		{
			name:            "invalid json",
			body:            `{"method":"GET","url":"https://example.com"`,
			wantStatus:      http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name: "domain error maps to bad request",
			body: `{"method":"TRACE","url":"https://example.com","headers":{}}`,
			createTaskFn: func(_ context.Context, _ service.CreateInput) (string, error) {
				return "", task.ErrInvalidMethod
			},
			wantStatus:      http.StatusBadRequest,
			wantContentType: "application/json",
			wantJSONBody:    true,
			wantErrorSubstr: task.ErrInvalidMethod.Error(),
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			apiHandler := NewHandler(&mockTaskService{createTaskFn: tc.createTaskFn}, nil, "9091", newTestLogger())
			router := NewRouter(apiHandler)

			req := httptest.NewRequest(http.MethodPost, "/task", bytes.NewBufferString(tc.body))
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			require.Equal(t, tc.wantStatus, rec.Code, "status code mismatch")
			require.Equal(t, tc.wantContentType, rec.Header().Get("Content-Type"), "content-type mismatch")

			if !tc.wantJSONBody {
				return
			}

			var payload map[string]any
			err := json.Unmarshal(rec.Body.Bytes(), &payload)
			require.NoError(t, err, "failed to decode response body")

			if tc.wantID != "" {
				gotID, _ := payload["id"].(string)
				require.Equal(t, tc.wantID, gotID, "id mismatch")
			}

			if tc.wantErrorSubstr != "" {
				gotErr, _ := payload["error"].(string)
				require.Contains(t, gotErr, tc.wantErrorSubstr, "error mismatch")
			}
		})
	}
}

func TestHandlersGetTaskNotFound(t *testing.T) {
	t.Parallel()

	apiHandler := NewHandler(&mockTaskService{
		getTaskFn: func(_ context.Context, _ string) (task.Task, error) {
			return task.Task{}, task.ErrTaskNotFound
		},
	}, nil, "9091", newTestLogger())
	router := NewRouter(apiHandler)

	req := httptest.NewRequest(http.MethodGet, "/task/missing-id", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code, "status code mismatch")

	var payload map[string]any
	err := json.Unmarshal(rec.Body.Bytes(), &payload)
	require.NoError(t, err, "failed to decode response body")

	gotErr, _ := payload["error"].(string)
	require.Equal(t, task.ErrTaskNotFound.Error(), gotErr, "error mismatch")
}

func TestHandlersHealthzServiceUnavailable(t *testing.T) {
	t.Parallel()

	apiHandler := NewHandler(&mockTaskService{}, mockHealthChecker{pingErr: errors.New("db is down")}, "9091", newTestLogger())
	router := NewRouter(apiHandler)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code, "status code mismatch")

	var payload map[string]any
	err := json.Unmarshal(rec.Body.Bytes(), &payload)
	require.NoError(t, err, "failed to decode response body")

	got, _ := payload["status"].(string)
	require.Equal(t, "unavailable", got, "health status mismatch")
}

func TestHandlersHealthzOk(t *testing.T) {
	t.Parallel()

	apiHandler := NewHandler(&mockTaskService{}, mockHealthChecker{pingErr: nil}, "9091", newTestLogger())
	router := NewRouter(apiHandler)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "status code mismatch")

	var payload map[string]any
	err := json.Unmarshal(rec.Body.Bytes(), &payload)
	require.NoError(t, err, "failed to decode response body")

	got, _ := payload["status"].(string)
	require.Equal(t, "ok", got, "health status mismatch")
}

func TestHandlersHealthzNoCheckerUnavailable(t *testing.T) {
	t.Parallel()

	apiHandler := NewHandler(&mockTaskService{}, nil, "9091", newTestLogger())
	router := NewRouter(apiHandler)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code, "status code mismatch")

	var payload map[string]any
	err := json.Unmarshal(rec.Body.Bytes(), &payload)
	require.NoError(t, err, "failed to decode response body")

	got, _ := payload["status"].(string)
	require.Equal(t, "unavailable", got, "health status mismatch")
}

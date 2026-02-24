//go:build integration

package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	httpopenapi "todoapp/internal/delivery/http/openapi"
	domaintask "todoapp/internal/domain/task"
	postgresrepo "todoapp/internal/repository/postgres"
	taskservice "todoapp/internal/usecase/task"

	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/require"
	testcontainers "github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/gorm"
)

type routerIntegrationEnv struct {
	router   http.Handler
	store    *postgresrepo.Store
	db       *gorm.DB
	executor *threadSafeFakeExecutor
}

func newRouterIntegrationEnv(t *testing.T, executor *threadSafeFakeExecutor) *routerIntegrationEnv {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	require.NotNil(t, executor, "executor is nil")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)

	container, err := tcpostgres.Run(
		ctx,
		"postgres:17-alpine",
		tcpostgres.WithDatabase("todoapp_test"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategyAndDeadline(
			3*time.Minute,
			wait.ForSQL("5432/tcp", "pgx", func(host string, port nat.Port) string {
				return fmt.Sprintf(
					"postgres://postgres:postgres@%s:%s/todoapp_test?sslmode=disable",
					host,
					port.Port(),
				)
			}).WithStartupTimeout(3*time.Minute),
		),
	)
	require.NoError(t, err, "start postgres container")

	t.Cleanup(func() {
		terminateCtx, terminateCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer terminateCancel()
		if err := container.Terminate(terminateCtx); err != nil {
			t.Logf("terminate postgres container: %v", err)
		}
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err, "build postgres dsn")

	gormDB, sqlDB, err := postgresrepo.NewPool(ctx, dsn)
	require.NoError(t, err, "connect to postgres")
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Logf("close sql db: %v", err)
		}
	})

	err = postgresrepo.RunMigrations(ctx, gormDB)
	require.NoError(t, err, "run migrations")

	store := postgresrepo.NewStore(gormDB)
	svc := taskservice.NewService(store, executor, store)
	apiHandler := NewHandler(svc, store, "9091", newTestLogger())

	env := &routerIntegrationEnv{
		router:   NewRouter(apiHandler),
		store:    store,
		db:       gormDB,
		executor: executor,
	}
	env.resetTasks(t)

	return env
}

func (e *routerIntegrationEnv) resetTasks(t *testing.T) {
	t.Helper()

	err := e.db.Exec("TRUNCATE TABLE tasks").Error
	require.NoError(t, err, "truncate tasks table")
}

type executorCall struct {
	Method  string
	URL     string
	Headers map[string]string
}

type threadSafeFakeExecutor struct {
	mu      sync.Mutex
	result  taskservice.ExecuteResult
	err     error
	calls   []executorCall
	calledC chan struct{}
	once    sync.Once
}

func newThreadSafeFakeExecutor(result taskservice.ExecuteResult, err error) *threadSafeFakeExecutor {
	return &threadSafeFakeExecutor{
		result:  cloneExecuteResult(result),
		err:     err,
		calls:   make([]executorCall, 0, 1),
		calledC: make(chan struct{}),
	}
}

func (f *threadSafeFakeExecutor) Execute(_ context.Context, method, url string, headers map[string]string) (taskservice.ExecuteResult, error) {
	f.mu.Lock()
	f.calls = append(f.calls, executorCall{
		Method:  method,
		URL:     url,
		Headers: cloneStringMap(headers),
	})
	result := cloneExecuteResult(f.result)
	err := f.err
	f.mu.Unlock()

	f.once.Do(func() {
		close(f.calledC)
	})

	return result, err
}

func (f *threadSafeFakeExecutor) WaitCalled(timeout time.Duration) bool {
	select {
	case <-f.calledC:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (f *threadSafeFakeExecutor) LastCall() (executorCall, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.calls) == 0 {
		return executorCall{}, false
	}

	last := f.calls[len(f.calls)-1]
	last.Headers = cloneStringMap(last.Headers)
	return last, true
}

func TestRouterIntegration_CreateThenGetTask(t *testing.T) {
	executor := newThreadSafeFakeExecutor(taskservice.ExecuteResult{
		HTTPStatusCode: 200,
		Headers: map[string][]string{
			"Content-Type": {"application/json"},
			"X-Source":     {"fake-executor"},
		},
		Length: 321,
	}, nil)
	env := newRouterIntegrationEnv(t, executor)

	taskID := mustCreateTask(t, env.router, httpopenapi.CreateTaskRequest{
		Method: "GET",
		Url:    "https://example.com/resource",
		Headers: &map[string]string{
			"Authorization": "Bearer t",
		},
	})

	require.True(t, executor.WaitCalled(2*time.Second), "executor was not called within timeout")

	call, ok := executor.LastCall()
	require.True(t, ok, "executor call was not recorded")
	require.Equal(t, "GET", call.Method, "executor method mismatch")
	require.Equal(t, "https://example.com/resource", call.URL, "executor url mismatch")
	require.Equal(t, "Bearer t", call.Headers["Authorization"], "executor header mismatch")

	getResp := waitForTaskStatus(t, env.router, taskID, httpopenapi.Done, 5*time.Second)

	require.NotNil(t, getResp.Id, "get task id is nil")
	require.Equal(t, taskID, *getResp.Id, "get task id mismatch")
	require.NotNil(t, getResp.HttpStatusCode, "get task httpStatusCode is nil")
	require.Equal(t, 200, *getResp.HttpStatusCode, "get task httpStatusCode mismatch")
	require.NotNil(t, getResp.Length, "get task length is nil")
	require.Equal(t, int64(321), *getResp.Length, "get task length mismatch")
	require.NotNil(t, getResp.Headers, "get task headers is nil")
	wantHeaders := map[string][]string{
		"Content-Type": {"application/json"},
		"X-Source":     {"fake-executor"},
	}
	require.Equal(t, wantHeaders, *getResp.Headers, "get task headers mismatch")
}

func TestRouterIntegration_CreateMultiplyThenGetTasks(t *testing.T) {
	executor := newThreadSafeFakeExecutor(taskservice.ExecuteResult{
		HTTPStatusCode: 200,
		Headers: map[string][]string{
			"Content-Type": {"application/json"},
			"X-Source":     {"fake-executor"},
		},
		Length: 321,
	}, nil)
	env := newRouterIntegrationEnv(t, executor)

	firstTaskID := mustCreateTask(t, env.router, httpopenapi.CreateTaskRequest{
		Method: "GET",
		Url:    "https://example.com/resource",
		Headers: &map[string]string{
			"Authorization": "Bearer t",
		},
	})
	secondTaskID := mustCreateTask(t, env.router, httpopenapi.CreateTaskRequest{
		Method: "GET",
		Url:    "https://example.com/resource",
		Headers: &map[string]string{
			"Authorization": "Bearer t",
		},
	})

	require.True(t, executor.WaitCalled(2*time.Second), "executor was not called within timeout")

	call, ok := executor.LastCall()
	require.True(t, ok, "executor call was not recorded")
	require.Equal(t, "GET", call.Method, "executor method mismatch")
	require.Equal(t, "https://example.com/resource", call.URL, "executor url mismatch")
	require.Equal(t, "Bearer t", call.Headers["Authorization"], "executor header mismatch")

	_ = waitForTaskStatus(t, env.router, firstTaskID, httpopenapi.Done, 5*time.Second)
	_ = waitForTaskStatus(t, env.router, secondTaskID, httpopenapi.Done, 5*time.Second)

	getAllResp := mustGetAllTasks(t, env.router)
	require.NotNil(t, getAllResp.Tasks, "get all tasks response has nil tasks")
	require.Len(t, *getAllResp.Tasks, 2, "tasks count mismatch")

	byID := make(map[string]httpopenapi.GetTaskResponse, len(*getAllResp.Tasks))
	for _, item := range *getAllResp.Tasks {
		require.NotNil(t, item.Id, "task in list has nil id")
		byID[*item.Id] = item
	}

	first, ok := byID[firstTaskID]
	require.True(t, ok, "first task not found in /tasks response")
	second, ok := byID[secondTaskID]
	require.True(t, ok, "second task not found in /tasks response")

	assertListedTaskDone(t, first)
	assertListedTaskDone(t, second)

	wantHeaders := map[string][]string{
		"Content-Type": {"application/json"},
		"X-Source":     {"fake-executor"},
	}
	require.Equal(t, wantHeaders, *first.Headers, "first task headers mismatch")
	require.Equal(t, wantHeaders, *second.Headers, "second task headers mismatch")
}

func TestRouterIntegration_CreateThenDeleteTask(t *testing.T) {
	executor := newThreadSafeFakeExecutor(taskservice.ExecuteResult{
		HTTPStatusCode: 200,
		Headers: map[string][]string{
			"Content-Type": {"application/json"},
			"X-Source":     {"fake-executor"},
		},
		Length: 321,
	}, nil)
	env := newRouterIntegrationEnv(t, executor)

	taskID := mustCreateTask(t, env.router, httpopenapi.CreateTaskRequest{
		Method: "GET",
		Url:    "https://example.com/resource",
		Headers: &map[string]string{
			"Authorization": "Bearer t",
		},
	})

	require.True(t, executor.WaitCalled(2*time.Second), "executor was not called within timeout")

	call, ok := executor.LastCall()
	require.True(t, ok, "executor call was not recorded")
	require.Equal(t, "GET", call.Method, "executor method mismatch")
	require.Equal(t, "https://example.com/resource", call.URL, "executor url mismatch")
	require.Equal(t, "Bearer t", call.Headers["Authorization"], "executor header mismatch")

	_ = waitForTaskStatus(t, env.router, taskID, httpopenapi.Done, 5*time.Second)

	deleteResp := mustDeleteTask(t, env.router, taskID)
	require.NotNil(t, deleteResp.Id, "delete task id is nil")
	require.Equal(t, taskID, *deleteResp.Id, "delete task id mismatch")
	require.NotNil(t, deleteResp.HttpStatusCode, "delete task httpStatusCode is nil")
	require.Equal(t, 200, *deleteResp.HttpStatusCode, "delete task httpStatusCode mismatch")

	getAfterDeleteRec := mustDoRequest(t, env.router, http.MethodGet, "/task/"+taskID, nil)
	require.Equal(t, http.StatusNotFound, getAfterDeleteRec.Code, "get after delete status mismatch: body=%s", getAfterDeleteRec.Body.String())

	errResp := mustDecodeJSON[httpopenapi.ErrorResponse](t, getAfterDeleteRec)
	require.NotNil(t, errResp.Error, "get after delete error is nil")
	require.Equal(t, domaintask.ErrTaskNotFound.Error(), *errResp.Error, "get after delete error mismatch")
}

func waitForTaskStatus(t *testing.T, router http.Handler, id string, want httpopenapi.GetTaskResponseStatus, timeout time.Duration) httpopenapi.GetTaskResponse {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var lastResp httpopenapi.GetTaskResponse
	var lastStatus string

	for time.Now().Before(deadline) {
		rec := mustDoRequest(t, router, http.MethodGet, "/task/"+id, nil)

		require.Equal(t, http.StatusOK, rec.Code, "unexpected get task status while polling: body=%s", rec.Body.String())

		resp := mustDecodeJSON[httpopenapi.GetTaskResponse](t, rec)

		lastResp = resp
		if resp.Status != nil {
			lastStatus = string(*resp.Status)
			if *resp.Status == want {
				return resp
			}
		}

		time.Sleep(50 * time.Millisecond)
	}

	require.Failf(t, "timeout waiting for task status", "task=%q want=%q last_status=%q last_response=%+v", id, want, lastStatus, lastResp)
	return httpopenapi.GetTaskResponse{}
}

func mustCreateTask(t *testing.T, router http.Handler, reqBody httpopenapi.CreateTaskRequest) string {
	t.Helper()

	rec := mustDoJSONRequest(t, router, http.MethodPost, "/task", reqBody)
	require.Equal(t, http.StatusOK, rec.Code, "create task status mismatch: body=%s", rec.Body.String())

	resp := mustDecodeJSON[httpopenapi.CreateTaskResponse](t, rec)
	require.NotNil(t, resp.Id, "create response id is nil")
	require.NotEmpty(t, *resp.Id, "create response id is empty")

	return *resp.Id
}

func mustDeleteTask(t *testing.T, router http.Handler, taskID string) httpopenapi.DeleteTaskResponse {
	t.Helper()

	rec := mustDoRequest(t, router, http.MethodDelete, "/task/"+taskID, nil)
	require.Equal(t, http.StatusOK, rec.Code, "delete task status mismatch: body=%s", rec.Body.String())

	return mustDecodeJSON[httpopenapi.DeleteTaskResponse](t, rec)
}

func mustGetAllTasks(t *testing.T, router http.Handler) httpopenapi.GetAllTasksResponse {
	t.Helper()

	rec := mustDoRequest(t, router, http.MethodGet, "/tasks", nil)
	require.Equal(t, http.StatusOK, rec.Code, "get all tasks status mismatch: body=%s", rec.Body.String())

	return mustDecodeJSON[httpopenapi.GetAllTasksResponse](t, rec)
}

func assertListedTaskDone(t *testing.T, resp httpopenapi.GetTaskResponse) {
	t.Helper()

	require.NotNil(t, resp.Status, "listed task status is nil")
	require.Equal(t, httpopenapi.Done, *resp.Status, "listed task status mismatch")
	require.NotNil(t, resp.HttpStatusCode, "listed task httpStatusCode is nil")
	require.Equal(t, 200, *resp.HttpStatusCode, "listed task httpStatusCode mismatch")
	require.NotNil(t, resp.Length, "listed task length is nil")
	require.Equal(t, int64(321), *resp.Length, "listed task length mismatch")
	require.NotNil(t, resp.Headers, "listed task headers is nil")
}

func mustDoJSONRequest(t *testing.T, router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	payload, err := json.Marshal(body)
	require.NoError(t, err, "marshal request body")

	return mustDoRequest(t, router, method, path, payload)
}

func mustDoRequest(t *testing.T, router http.Handler, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()

	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}

	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func mustDecodeJSON[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()

	var out T
	err := json.Unmarshal(rec.Body.Bytes(), &out)
	require.NoError(t, err, "decode json response: body=%s", rec.Body.String())

	return out
}

func cloneExecuteResult(in taskservice.ExecuteResult) taskservice.ExecuteResult {
	out := in
	out.Headers = cloneStringSliceMap(in.Headers)
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}

	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneStringSliceMap(in map[string][]string) map[string][]string {
	if len(in) == 0 {
		return map[string][]string{}
	}

	out := make(map[string][]string, len(in))
	for k, values := range in {
		copied := make([]string, len(values))
		copy(copied, values)
		out[k] = copied
	}
	return out
}

var _ taskservice.Executor = (*threadSafeFakeExecutor)(nil)

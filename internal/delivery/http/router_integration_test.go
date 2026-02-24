//go:build integration

package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	httpopenapi "todoapp/internal/delivery/http/openapi"
	domaintask "todoapp/internal/domain/task"
	postgresrepo "todoapp/internal/repository/postgres"
	taskservice "todoapp/internal/usecase/task"

	"github.com/docker/go-connections/nat"
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
	if executor == nil {
		t.Fatalf("executor is nil")
	}

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
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}

	t.Cleanup(func() {
		terminateCtx, terminateCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer terminateCancel()
		if err := container.Terminate(terminateCtx); err != nil {
			t.Logf("terminate postgres container: %v", err)
		}
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("build postgres dsn: %v", err)
	}

	gormDB, sqlDB, err := postgresrepo.NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to postgres: %v", err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Logf("close sql db: %v", err)
		}
	})

	if err := postgresrepo.RunMigrations(ctx, gormDB); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

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

	if err := e.db.Exec("TRUNCATE TABLE tasks").Error; err != nil {
		t.Fatalf("truncate tasks table: %v", err)
	}
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

	if !executor.WaitCalled(2 * time.Second) {
		t.Fatalf("executor was not called within timeout")
	}

	call, ok := executor.LastCall()
	if !ok {
		t.Fatalf("executor call was not recorded")
	}
	if call.Method != "GET" {
		t.Fatalf("executor method mismatch: got %q, want %q", call.Method, "GET")
	}
	if call.URL != "https://example.com/resource" {
		t.Fatalf("executor url mismatch: got %q, want %q", call.URL, "https://example.com/resource")
	}
	if got, want := call.Headers["Authorization"], "Bearer t"; got != want {
		t.Fatalf("executor header mismatch: got %q, want %q", got, want)
	}

	getResp := waitForTaskStatus(t, env.router, taskID, httpopenapi.Done, 5*time.Second)

	if getResp.Id == nil || *getResp.Id != taskID {
		t.Fatalf("get task id mismatch: got %+v, want id %q", getResp.Id, taskID)
	}
	if getResp.HttpStatusCode == nil || *getResp.HttpStatusCode != 200 {
		t.Fatalf("get task httpStatusCode mismatch: got %+v, want %d", getResp.HttpStatusCode, 200)
	}
	if getResp.Length == nil || *getResp.Length != 321 {
		t.Fatalf("get task length mismatch: got %+v, want %d", getResp.Length, 321)
	}
	if getResp.Headers == nil {
		t.Fatalf("get task headers is nil")
	}
	wantHeaders := map[string][]string{
		"Content-Type": {"application/json"},
		"X-Source":     {"fake-executor"},
	}
	if !reflect.DeepEqual(*getResp.Headers, wantHeaders) {
		t.Fatalf("get task headers mismatch:\n got: %#v\nwant: %#v", *getResp.Headers, wantHeaders)
	}
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

	if !executor.WaitCalled(2 * time.Second) {
		t.Fatalf("executor was not called within timeout")
	}

	call, ok := executor.LastCall()
	if !ok {
		t.Fatalf("executor call was not recorded")
	}
	if call.Method != "GET" {
		t.Fatalf("executor method mismatch: got %q, want %q", call.Method, "GET")
	}
	if call.URL != "https://example.com/resource" {
		t.Fatalf("executor url mismatch: got %q, want %q", call.URL, "https://example.com/resource")
	}
	if got, want := call.Headers["Authorization"], "Bearer t"; got != want {
		t.Fatalf("executor header mismatch: got %q, want %q", got, want)
	}

	_ = waitForTaskStatus(t, env.router, firstTaskID, httpopenapi.Done, 5*time.Second)
	_ = waitForTaskStatus(t, env.router, secondTaskID, httpopenapi.Done, 5*time.Second)

	getAllResp := mustGetAllTasks(t, env.router)
	if getAllResp.Tasks == nil {
		t.Fatalf("get all tasks response has nil tasks")
	}
	if len(*getAllResp.Tasks) != 2 {
		t.Fatalf("tasks count mismatch: got %d, want %d", len(*getAllResp.Tasks), 2)
	}

	byID := make(map[string]httpopenapi.GetTaskResponse, len(*getAllResp.Tasks))
	for _, item := range *getAllResp.Tasks {
		if item.Id == nil {
			t.Fatalf("task in list has nil id: %+v", item)
		}
		byID[*item.Id] = item
	}

	first, ok := byID[firstTaskID]
	if !ok {
		t.Fatalf("first task not found in /tasks response: %q", firstTaskID)
	}
	second, ok := byID[secondTaskID]
	if !ok {
		t.Fatalf("second task not found in /tasks response: %q", secondTaskID)
	}

	assertListedTaskDone(t, first)
	assertListedTaskDone(t, second)

	wantHeaders := map[string][]string{
		"Content-Type": {"application/json"},
		"X-Source":     {"fake-executor"},
	}
	if !reflect.DeepEqual(*first.Headers, wantHeaders) {
		t.Fatalf("first task headers mismatch:\n got: %#v\nwant: %#v", *first.Headers, wantHeaders)
	}
	if !reflect.DeepEqual(*second.Headers, wantHeaders) {
		t.Fatalf("second task headers mismatch:\n got: %#v\nwant: %#v", *second.Headers, wantHeaders)
	}
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

	if !executor.WaitCalled(2 * time.Second) {
		t.Fatalf("executor was not called within timeout")
	}

	call, ok := executor.LastCall()
	if !ok {
		t.Fatalf("executor call was not recorded")
	}
	if call.Method != "GET" {
		t.Fatalf("executor method mismatch: got %q, want %q", call.Method, "GET")
	}
	if call.URL != "https://example.com/resource" {
		t.Fatalf("executor url mismatch: got %q, want %q", call.URL, "https://example.com/resource")
	}
	if got, want := call.Headers["Authorization"], "Bearer t"; got != want {
		t.Fatalf("executor header mismatch: got %q, want %q", got, want)
	}

	_ = waitForTaskStatus(t, env.router, taskID, httpopenapi.Done, 5*time.Second)

	deleteResp := mustDeleteTask(t, env.router, taskID)
	if deleteResp.Id == nil || *deleteResp.Id != taskID {
		t.Fatalf("delete task id mismatch: got %+v, want id %q", deleteResp.Id, taskID)
	}
	if deleteResp.HttpStatusCode == nil || *deleteResp.HttpStatusCode != 200 {
		t.Fatalf("delete task httpStatusCode mismatch: got %+v, want %d", deleteResp.HttpStatusCode, 200)
	}

	getAfterDeleteRec := mustDoRequest(t, env.router, http.MethodGet, "/task/"+taskID, nil)
	if getAfterDeleteRec.Code != http.StatusNotFound {
		t.Fatalf("get after delete status mismatch: got %d, want %d; body=%s", getAfterDeleteRec.Code, http.StatusNotFound, getAfterDeleteRec.Body.String())
	}

	errResp := mustDecodeJSON[httpopenapi.ErrorResponse](t, getAfterDeleteRec)
	if errResp.Error == nil || *errResp.Error != domaintask.ErrTaskNotFound.Error() {
		t.Fatalf("get after delete error mismatch: got %+v, want %q", errResp.Error, domaintask.ErrTaskNotFound.Error())
	}
}

func waitForTaskStatus(t *testing.T, router http.Handler, id string, want httpopenapi.GetTaskResponseStatus, timeout time.Duration) httpopenapi.GetTaskResponse {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var lastResp httpopenapi.GetTaskResponse
	var lastStatus string

	for time.Now().Before(deadline) {
		rec := mustDoRequest(t, router, http.MethodGet, "/task/"+id, nil)

		if rec.Code != http.StatusOK {
			t.Fatalf("unexpected get task status while polling: got %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}

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

	t.Fatalf("timeout waiting for task %q status %q (last status=%q, last response=%+v)", id, want, lastStatus, lastResp)
	return httpopenapi.GetTaskResponse{}
}

func mustCreateTask(t *testing.T, router http.Handler, reqBody httpopenapi.CreateTaskRequest) string {
	t.Helper()

	rec := mustDoJSONRequest(t, router, http.MethodPost, "/task", reqBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("create task status mismatch: got %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	resp := mustDecodeJSON[httpopenapi.CreateTaskResponse](t, rec)
	if resp.Id == nil || *resp.Id == "" {
		t.Fatalf("create response id is empty: %+v", resp)
	}

	return *resp.Id
}

func mustDeleteTask(t *testing.T, router http.Handler, taskID string) httpopenapi.DeleteTaskResponse {
	t.Helper()

	rec := mustDoRequest(t, router, http.MethodDelete, "/task/"+taskID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete task status mismatch: got %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	return mustDecodeJSON[httpopenapi.DeleteTaskResponse](t, rec)
}

func mustGetAllTasks(t *testing.T, router http.Handler) httpopenapi.GetAllTasksResponse {
	t.Helper()

	rec := mustDoRequest(t, router, http.MethodGet, "/tasks", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get all tasks status mismatch: got %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	return mustDecodeJSON[httpopenapi.GetAllTasksResponse](t, rec)
}

func assertListedTaskDone(t *testing.T, resp httpopenapi.GetTaskResponse) {
	t.Helper()

	if resp.Status == nil || *resp.Status != httpopenapi.Done {
		t.Fatalf("listed task status mismatch: got %+v, want %q", resp.Status, httpopenapi.Done)
	}
	if resp.HttpStatusCode == nil || *resp.HttpStatusCode != 200 {
		t.Fatalf("listed task httpStatusCode mismatch: got %+v, want %d", resp.HttpStatusCode, 200)
	}
	if resp.Length == nil || *resp.Length != 321 {
		t.Fatalf("listed task length mismatch: got %+v, want %d", resp.Length, 321)
	}
	if resp.Headers == nil {
		t.Fatalf("listed task headers is nil")
	}
}

func mustDoJSONRequest(t *testing.T, router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}

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
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode json response: %v; body=%s", err, rec.Body.String())
	}

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

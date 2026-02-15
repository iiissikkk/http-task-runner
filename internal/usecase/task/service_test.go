package service

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"todoapp/internal/domain/task"
)

type fakeExecutor struct {
	result ExecuteResult
	err    error
	called bool
}

func (e *fakeExecutor) Execute(_ context.Context, _ string, _ string, _ map[string]string) (ExecuteResult, error) {
	e.called = true
	return e.result, e.err
}

type fakeStore struct {
	mu            sync.RWMutex
	tasks         map[string]task.Task
	updateHistory []task.Status
}

var _ Store = (*fakeStore)(nil)

func newFakeStore() *fakeStore {
	return &fakeStore{
		tasks:         make(map[string]task.Task),
		updateHistory: make([]task.Status, 0),
	}
}

func (s *fakeStore) Create(_ context.Context, item task.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[item.ID] = cloneTask(item)
	return nil
}

func (s *fakeStore) GetByID(_ context.Context, id string) (task.Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	item, ok := s.tasks[id]
	if !ok {
		return task.Task{}, task.ErrTaskNotFound
	}

	return cloneTask(item), nil
}

func (s *fakeStore) GetAll(_ context.Context) ([]task.Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]task.Task, 0, len(s.tasks))
	for _, item := range s.tasks {
		items = append(items, cloneTask(item))
	}

	return items, nil
}

func (s *fakeStore) Update(_ context.Context, item task.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.tasks[item.ID]; !ok {
		return task.ErrTaskNotFound
	}

	s.tasks[item.ID] = cloneTask(item)
	s.updateHistory = append(s.updateHistory, item.Status)
	return nil
}

func (s *fakeStore) Delete(_ context.Context, id string) (task.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item, ok := s.tasks[id]
	if !ok {
		return task.Task{}, task.ErrTaskNotFound
	}

	delete(s.tasks, id)
	return cloneTask(item), nil
}

func cloneTask(item task.Task) task.Task {
	out := item

	if item.RequestHeaders != nil {
		out.RequestHeaders = make(map[string]string, len(item.RequestHeaders))
		for k, v := range item.RequestHeaders {
			out.RequestHeaders[k] = v
		}
	}

	if item.Headers != nil {
		out.Headers = make(map[string][]string, len(item.Headers))
		for k, values := range item.Headers {
			copied := make([]string, len(values))
			copy(copied, values)
			out.Headers[k] = copied
		}
	}

	return out
}

func TestServiceCreateTaskValidation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		input   CreateInput
		wantErr error
	}{
		{
			name: "method is required",
			input: CreateInput{
				Method:  "",
				URL:     "https://example.com",
				Headers: map[string]string{},
			},
			wantErr: task.ErrMethodRequired,
		},
		{
			name: "url is required",
			input: CreateInput{
				Method:  "GET",
				URL:     "",
				Headers: map[string]string{},
			},
			wantErr: task.ErrURLRequired,
		},
		{
			name: "invalid method",
			input: CreateInput{
				Method:  "TRACE",
				URL:     "https://example.com",
				Headers: map[string]string{},
			},
			wantErr: task.ErrInvalidMethod,
		},
		{
			name: "invalid url",
			input: CreateInput{
				Method:  "GET",
				URL:     "://bad-url",
				Headers: map[string]string{},
			},
			wantErr: task.ErrInvalidURL,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := newFakeStore()
			svc := NewService(store, &fakeExecutor{})

			_, err := svc.CreateTask(context.Background(), tc.input)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error mismatch: got %v, want %v", err, tc.wantErr)
			}

			if len(store.tasks) != 0 {
				t.Fatalf("unexpected tasks in store: got %d, want 0", len(store.tasks))
			}
		})
	}
}

func TestServiceRunTaskSuccess(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.tasks["task-1"] = task.Task{
		ID:             "task-1",
		Method:         "GET",
		URL:            "https://example.com",
		RequestHeaders: map[string]string{"Authorization": "token"},
		Status:         task.StatusNew,
		Headers:        map[string][]string{},
	}

	executor := &fakeExecutor{
		result: ExecuteResult{
			HTTPStatusCode: 200,
			Headers: map[string][]string{
				"Content-Type": {"application/json"},
			},
			Length: 123,
		},
	}

	svc := NewService(store, executor)
	svc.runTask("task-1")

	got, err := store.GetByID(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("failed to get task from store: %v", err)
	}

	if got.Status != task.StatusDone {
		t.Fatalf("status mismatch: got %q, want %q", got.Status, task.StatusDone)
	}
	if got.HTTPStatusCode != 200 {
		t.Fatalf("http status mismatch: got %d, want %d", got.HTTPStatusCode, 200)
	}
	if got.Length != 123 {
		t.Fatalf("length mismatch: got %d, want %d", got.Length, 123)
	}
	if !executor.called {
		t.Fatalf("executor was not called")
	}

	wantHistory := []task.Status{task.StatusInProcess, task.StatusDone}
	if !reflect.DeepEqual(store.updateHistory, wantHistory) {
		t.Fatalf("update history mismatch: got %v, want %v", store.updateHistory, wantHistory)
	}
}

func TestServiceRunTaskExecutorError(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.tasks["task-1"] = task.Task{
		ID:             "task-1",
		Method:         "GET",
		URL:            "https://example.com",
		RequestHeaders: map[string]string{"Authorization": "token"},
		Status:         task.StatusNew,
		Headers:        map[string][]string{},
	}

	executor := &fakeExecutor{err: errors.New("request failed")}
	svc := NewService(store, executor)
	svc.runTask("task-1")

	got, err := store.GetByID(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("failed to get task from store: %v", err)
	}

	if got.Status != task.StatusError {
		t.Fatalf("status mismatch: got %q, want %q", got.Status, task.StatusError)
	}
	if !executor.called {
		t.Fatalf("executor was not called")
	}

	wantHistory := []task.Status{task.StatusInProcess, task.StatusError}
	if !reflect.DeepEqual(store.updateHistory, wantHistory) {
		t.Fatalf("update history mismatch: got %v, want %v", store.updateHistory, wantHistory)
	}
}

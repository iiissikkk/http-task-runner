package service

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"todoapp/internal/domain/task"
)

type Store interface {
	Create(ctx context.Context, task task.Task) error
	GetByID(ctx context.Context, id string) (task.Task, error)
	GetAll(ctx context.Context) ([]task.Task, error)
	Update(ctx context.Context, task task.Task) error
	Delete(ctx context.Context, id string) (task.Task, error)
}

type ExecuteResult struct {
	HTTPStatusCode int
	Headers        map[string][]string
	Length         int64
}

type Executor interface {
	Execute(ctx context.Context, method string, url string, headers map[string]string) (ExecuteResult, error)
}

type Service struct {
	store    Store
	executor Executor
	idFn     func() string
}

type CreateInput struct {
	Method  string
	URL     string
	Headers map[string]string
}

func NewService(store Store, executor Executor) *Service {
	return &Service{
		store:    store,
		executor: executor,
		idFn: func() string {
			return uuid.NewString()
		},
	}
}

func (s *Service) CreateTask(ctx context.Context, input CreateInput) (string, error) {
	if err := validateInput(input); err != nil {
		return "", err
	}

	t := task.Task{
		ID:             s.idFn(),
		Method:         strings.ToUpper(strings.TrimSpace(input.Method)),
		URL:            strings.TrimSpace(input.URL),
		RequestHeaders: copyRequestHeaders(input.Headers),
		Status:         task.StatusNew,
		Headers:        map[string][]string{},
	}

	if err := s.store.Create(ctx, t); err != nil {
		return "", err
	}

	go s.runTask(t.ID)

	return t.ID, nil
}

func (s *Service) GetTask(ctx context.Context, id string) (task.Task, error) {
	return s.store.GetByID(ctx, id)
}

func (s *Service) GetAllTasks(ctx context.Context) ([]task.Task, error) {
	return s.store.GetAll(ctx)
}

func (s *Service) DeleteTask(ctx context.Context, id string) (task.Task, error) {
	return s.store.Delete(ctx, id)
}

func (s *Service) runTask(taskID string) {
	ctx := context.Background()

	t, err := s.store.GetByID(ctx, taskID)
	if err != nil {
		return
	}

	t.Status = task.StatusInProcess
	if err = s.store.Update(ctx, t); err != nil {
		return
	}

	result, err := s.executor.Execute(ctx, t.Method, t.URL, t.RequestHeaders)
	if err != nil {
		t.Status = task.StatusError
		_ = s.store.Update(ctx, t)
		return
	}

	t.Status = task.StatusDone
	t.HTTPStatusCode = result.HTTPStatusCode
	t.Headers = copyResponseHeaders(result.Headers)
	t.Length = result.Length
	_ = s.store.Update(ctx, t)
}

func validateInput(input CreateInput) error {
	if strings.TrimSpace(input.Method) == "" {
		return task.ErrMethodRequired
	}
	if strings.TrimSpace(input.URL) == "" {
		return task.ErrURLRequired
	}

	method := strings.ToUpper(strings.TrimSpace(input.Method))
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions:
	default:
		return task.ErrInvalidMethod
	}

	parsedURL, err := url.ParseRequestURI(strings.TrimSpace(input.URL))
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return task.ErrInvalidURL
	}

	return nil
}

func copyRequestHeaders(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}

	out := make(map[string]string, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func copyResponseHeaders(input map[string][]string) map[string][]string {
	if len(input) == 0 {
		return map[string][]string{}
	}

	out := make(map[string][]string, len(input))
	for k, values := range input {
		copied := make([]string, len(values))
		copy(copied, values)
		out[k] = copied
	}
	return out
}

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

type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
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
	store     Store
	executor  Executor
	txManager TxManager
	idFn      func() string
}

type CreateInput struct {
	Method  string
	URL     string
	Headers map[string]string
}

func NewService(store Store, executor Executor, txManager ...TxManager) *Service {
	var manager TxManager
	if len(txManager) > 0 {
		manager = txManager[0]
	}

	return &Service{
		store:     store,
		executor:  executor,
		txManager: manager,
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

	if err := s.withTx(ctx, func(txCtx context.Context) error {
		return s.store.Create(txCtx, t)
	}); err != nil {
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
	var item task.Task

	if err := s.withTx(ctx, func(txCtx context.Context) error {
		var err error
		item, err = s.store.Delete(txCtx, id)
		return err
	}); err != nil {
		return task.Task{}, err
	}

	return item, nil
}

func (s *Service) runTask(taskID string) {
	ctx := context.Background()

	t, err := s.store.GetByID(ctx, taskID)
	if err != nil {
		return
	}

	t.Status = task.StatusInProcess
	if err = s.withTx(ctx, func(txCtx context.Context) error {
		return s.store.Update(txCtx, t)
	}); err != nil {
		return
	}

	result, err := s.executor.Execute(ctx, t.Method, t.URL, t.RequestHeaders)
	if err != nil {
		t.Status = task.StatusError
		_ = s.withTx(ctx, func(txCtx context.Context) error {
			return s.store.Update(txCtx, t)
		})
		return
	}

	t.Status = task.StatusDone
	t.HTTPStatusCode = result.HTTPStatusCode
	t.Headers = copyResponseHeaders(result.Headers)
	t.Length = result.Length
	_ = s.withTx(ctx, func(txCtx context.Context) error {
		return s.store.Update(txCtx, t)
	})
}

func (s *Service) withTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if s.txManager == nil {
		return fn(ctx)
	}

	return s.txManager.WithinTx(ctx, fn)
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

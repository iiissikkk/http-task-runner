package store

import (
	"context"
	"sync"

	"todoapp/internal/domain/task"
)

type MemoryStore struct {
	mu    sync.RWMutex
	tasks map[string]task.Task
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{tasks: make(map[string]task.Task)}
}

func (s *MemoryStore) Create(_ context.Context, item task.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tasks[item.ID] = cloneTask(item)
	return nil
}

func (s *MemoryStore) GetByID(_ context.Context, id string) (task.Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	item, ok := s.tasks[id]
	if !ok {
		return task.Task{}, task.ErrTaskNotFound
	}

	return cloneTask(item), nil
}

func (s *MemoryStore) GetAll(_ context.Context) ([]task.Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]task.Task, 0, len(s.tasks))
	for _, v := range s.tasks {
		items = append(items, v)
	}
	return items, nil
}

func (s *MemoryStore) Delete(_ context.Context, id string) (task.Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	item, ok := s.tasks[id]
	if !ok {
		return task.Task{}, task.ErrTaskNotFound
	}

	delete(s.tasks, id)

	return item, nil
}

func (s *MemoryStore) Update(_ context.Context, item task.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.tasks[item.ID]; !ok {
		return task.ErrTaskNotFound
	}

	s.tasks[item.ID] = cloneTask(item)
	return nil
}

func cloneTask(item task.Task) task.Task {
	cloned := item

	if item.RequestHeaders != nil {
		cloned.RequestHeaders = make(map[string]string, len(item.RequestHeaders))
		for k, v := range item.RequestHeaders {
			cloned.RequestHeaders[k] = v
		}
	}

	if item.Headers != nil {
		cloned.Headers = make(map[string][]string, len(item.Headers))
		for k, values := range item.Headers {
			copied := make([]string, len(values))
			copy(copied, values)
			cloned.Headers[k] = copied
		}
	}

	return cloned
}

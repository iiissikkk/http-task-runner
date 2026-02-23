//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"todoapp/internal/domain/task"

	"github.com/docker/go-connections/nat"
	"github.com/google/uuid"
	testcontainers "github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

type storeIntegrationEnv struct {
	store *Store
}

func newStoreIntegrationEnv(t *testing.T) *storeIntegrationEnv {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)

	container, err := tcpostgres.Run(
		ctx,
		"postgres:16-alpine",
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

	gormDB, sqlDB, err := NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to postgres: %v", err)
	}

	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Logf("close sql db: %v", err)
		}
	})

	if err := RunMigrations(ctx, gormDB); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	env := &storeIntegrationEnv{store: NewStore(gormDB)}
	env.resetTasks(t)

	return env
}

func (e *storeIntegrationEnv) resetTasks(t *testing.T) {
	t.Helper()

	if err := e.store.db.Exec("TRUNCATE TABLE tasks").Error; err != nil {
		t.Fatalf("truncate tasks table: %v", err)
	}
}

func TestStoreIntegration_CreateAndGetByID(t *testing.T) {
	env := newStoreIntegrationEnv(t)
	ctx := context.Background()

	want := task.Task{
		ID:     uuid.NewString(),
		Method: "GET",
		URL:    "https://example.com/api",
		RequestHeaders: map[string]string{
			"Authorization": "Bearer test-token",
			"X-Trace-Id":    "trace-123",
		},
		Status:         task.StatusNew,
		HTTPStatusCode: 0,
		Headers:        map[string][]string{},
		Length:         0,
	}

	if err := env.store.Create(ctx, want); err != nil {
		t.Fatalf("create task: %v", err)
	}

	got, err := env.store.GetByID(ctx, want.ID)
	if err != nil {
		t.Fatalf("get task by id: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("task mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestStoreIntegration_GetAll(t *testing.T) {
	env := newStoreIntegrationEnv(t)
	ctx := context.Background()

	first := mustCreateTask(t, env, task.Task{
		ID:     "00000000-0000-0000-0000-000000000001",
		Method: "GET",
		URL:    "https://example.com/1",
		Status: task.StatusNew,
	})
	second := mustCreateTask(t, env, task.Task{
		ID:     "00000000-0000-0000-0000-000000000002",
		Method: "POST",
		URL:    "https://example.com/2",
		Status: task.StatusNew,
	})

	got, err := env.store.GetAll(ctx)
	if err != nil {
		t.Fatalf("get all tasks: %v", err)
	}

	want := []task.Task{first, second}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tasks mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestStoreIntegration_Update(t *testing.T) {
	env := newStoreIntegrationEnv(t)
	ctx := context.Background()

	want := mustCreateTask(t, env, task.Task{
		Method: "GET",
		URL:    "https://example.com",
		Status: task.StatusNew,
	})

	want.Status = task.StatusDone
	want.HTTPStatusCode = 200
	want.Headers = map[string][]string{"Content-Type": {"application/json"}}
	want.Length = 123

	if err := env.store.Update(ctx, want); err != nil {
		t.Fatalf("update task: %v", err)
	}

	got, err := env.store.GetByID(ctx, want.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("task mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestStoreIntegration_Delete(t *testing.T) {
	env := newStoreIntegrationEnv(t)
	ctx := context.Background()

	seed := mustCreateTask(t, env, task.Task{
		Method: "GET",
		URL:    "https://example.com",
		Status: task.StatusNew,
	})

	deleted, err := env.store.Delete(ctx, seed.ID)
	if err != nil {
		t.Fatalf("delete task: %v", err)
	}
	if !reflect.DeepEqual(deleted, seed) {
		t.Fatalf("deleted task mismatch:\n got: %#v\nwant: %#v", deleted, seed)
	}

	_, err = env.store.GetByID(ctx, seed.ID)
	if !errors.Is(err, task.ErrTaskNotFound) {
		t.Fatalf("expected not found after delete, got: %v", err)
	}
}

func TestStoreIntegration_WithinTxCommit(t *testing.T) {
	env := newStoreIntegrationEnv(t)
	ctx := context.Background()

	want := task.Task{
		ID:     uuid.NewString(),
		Method: "GET",
		URL:    "https://example.com/tx-commit",
		RequestHeaders: map[string]string{
			"X-Test": "1",
		},
		Status:         task.StatusNew,
		HTTPStatusCode: 0,
		Headers:        map[string][]string{},
		Length:         0,
	}

	if err := env.store.WithinTx(ctx, func(txCtx context.Context) error {
		return env.store.Create(txCtx, want)
	}); err != nil {
		t.Fatalf("within tx commit: %v", err)
	}

	got, err := env.store.GetByID(ctx, want.ID)
	if err != nil {
		t.Fatalf("get task after commit: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("task mismatch after commit:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestStoreIntegration_WithinTxRollback(t *testing.T) {
	env := newStoreIntegrationEnv(t)
	ctx := context.Background()

	item := task.Task{
		ID:      uuid.NewString(),
		Method:  "GET",
		URL:     "https://example.com/tx-rollback",
		Status:  task.StatusNew,
		Headers: map[string][]string{},
	}

	rollbackErr := errors.New("force rollback")

	err := env.store.WithinTx(ctx, func(txCtx context.Context) error {
		if err := env.store.Create(txCtx, item); err != nil {
			return err
		}

		// Inside the transaction the row should be visible via txCtx.
		if _, err := env.store.GetByID(txCtx, item.ID); err != nil {
			return err
		}

		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("expected wrapped rollback error, got: %v", err)
	}

	_, err = env.store.GetByID(ctx, item.ID)
	if !errors.Is(err, task.ErrTaskNotFound) {
		t.Fatalf("expected rolled back task to be absent, got: %v", err)
	}
}

func mustCreateTask(t *testing.T, env *storeIntegrationEnv, item task.Task) task.Task {
	t.Helper()

	if item.ID == "" {
		item.ID = uuid.NewString()
	}
	if item.Headers == nil {
		item.Headers = map[string][]string{}
	}
	if item.RequestHeaders == nil {
		item.RequestHeaders = map[string]string{}
	}

	if err := env.store.Create(context.Background(), item); err != nil {
		t.Fatalf("create task: %v", err)
	}
	return item
}

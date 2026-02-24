//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"todoapp/internal/domain/task"

	"github.com/docker/go-connections/nat"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
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

	gormDB, sqlDB, err := NewPool(ctx, dsn)
	require.NoError(t, err, "connect to postgres")

	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Logf("close sql db: %v", err)
		}
	})

	err = RunMigrations(ctx, gormDB)
	require.NoError(t, err, "run migrations")

	env := &storeIntegrationEnv{store: NewStore(gormDB)}
	env.resetTasks(t)

	return env
}

func (e *storeIntegrationEnv) resetTasks(t *testing.T) {
	t.Helper()

	err := e.store.db.Exec("TRUNCATE TABLE tasks").Error
	require.NoError(t, err, "truncate tasks table")
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

	err := env.store.Create(ctx, want)
	require.NoError(t, err, "create task")

	got, err := env.store.GetByID(ctx, want.ID)
	require.NoError(t, err, "get task by id")

	require.Equal(t, want, got, "task mismatch")
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
	require.NoError(t, err, "get all tasks")

	want := []task.Task{first, second}
	require.Equal(t, want, got, "tasks mismatch")
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

	err := env.store.Update(ctx, want)
	require.NoError(t, err, "update task")

	got, err := env.store.GetByID(ctx, want.ID)
	require.NoError(t, err, "get task")

	require.Equal(t, want, got, "task mismatch")
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
	require.NoError(t, err, "delete task")
	require.Equal(t, seed, deleted, "deleted task mismatch")

	_, err = env.store.GetByID(ctx, seed.ID)
	require.ErrorIs(t, err, task.ErrTaskNotFound, "expected not found after delete")
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

	err := env.store.WithinTx(ctx, func(txCtx context.Context) error {
		return env.store.Create(txCtx, want)
	})
	require.NoError(t, err, "within tx commit")

	got, err := env.store.GetByID(ctx, want.ID)
	require.NoError(t, err, "get task after commit")
	require.Equal(t, want, got, "task mismatch after commit")
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
	require.ErrorIs(t, err, rollbackErr, "expected wrapped rollback error")

	_, err = env.store.GetByID(ctx, item.ID)
	require.ErrorIs(t, err, task.ErrTaskNotFound, "expected rolled back task to be absent")
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

	err := env.store.Create(context.Background(), item)
	require.NoError(t, err, "create task")
	return item
}

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"todoapp/internal/domain/task"
	service "todoapp/internal/usecase/task"
)

type Store struct {
	pool *pgxpool.Pool
}

type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type txContextKey struct{}

var _ service.Store = (*Store)(nil)
var _ service.TxManager = (*Store)(nil)

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Ping(ctx context.Context) error {
	if s.pool == nil {
		return errors.New("postgres pool is nil")
	}
	return s.pool.Ping(ctx)
}

func (s *Store) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if s.pool == nil {
		return errors.New("postgres pool is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	txCtx := context.WithValue(ctx, txContextKey{}, tx)
	if err = fn(txCtx); err != nil {
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	committed = true
	return nil
}

func (s *Store) Create(ctx context.Context, item task.Task) error {
	db, err := s.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	reqHeadersJSON, err := marshalRequestHeaders(item.RequestHeaders)
	if err != nil {
		return err
	}

	respHeadersJSON, err := marshalResponseHeaders(item.Headers)
	if err != nil {
		return err
	}

	const query = `
		INSERT INTO tasks (
			id,
			method,
			url,
			status,
			request_headers,
			http_status_code,
			response_headers,
			length,
			created_at,
			updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	_, err = db.Exec(
		ctx,
		query,
		item.ID,
		item.Method,
		item.URL,
		string(item.Status),
		reqHeadersJSON,
		item.HTTPStatusCode,
		respHeadersJSON,
		item.Length,
		time.Now().UTC(),
		nil,
	)
	if err != nil {
		return fmt.Errorf("insert task: %w", err)
	}

	return nil
}

func (s *Store) GetByID(ctx context.Context, id string) (task.Task, error) {
	db, err := s.dbFromCtx(ctx)
	if err != nil {
		return task.Task{}, err
	}

	const query = `
			SELECT
				id,
				method,
				url,
				status,
				request_headers,
				http_status_code,
				response_headers,
				length
			FROM tasks
			WHERE id = $1
		`

	item, err := scanTaskRow(db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return task.Task{}, task.ErrTaskNotFound
		}
		return task.Task{}, fmt.Errorf("select task by id: %w", err)
	}

	return item, nil
}

func (s *Store) GetAll(ctx context.Context) ([]task.Task, error) {
	db, err := s.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	const query = `
			SELECT
				id,
				method,
				url,
				status,
				request_headers,
				http_status_code,
				response_headers,
				length
			FROM tasks
			ORDER BY created_at ASC, id ASC
		`

	rows, err := db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("select all tasks: %w", err)
	}
	defer rows.Close()

	items := make([]task.Task, 0)
	for rows.Next() {
		item, scanErr := scanTaskRow(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan task row: %w", scanErr)
		}
		items = append(items, item)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task rows: %w", err)
	}

	return items, nil
}

func (s *Store) Update(ctx context.Context, item task.Task) error {
	db, err := s.dbFromCtx(ctx)
	if err != nil {
		return err
	}

	respHeadersJSON, err := marshalResponseHeaders(item.Headers)
	if err != nil {
		return err
	}

	const query = `
		UPDATE tasks
		SET
			status = $2,
			http_status_code = $3,
			response_headers = $4,
			length = $5,
			updated_at = $6
		WHERE id = $1
	`

	tag, err := db.Exec(
		ctx,
		query,
		item.ID,
		string(item.Status),
		item.HTTPStatusCode,
		respHeadersJSON,
		item.Length,
		time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return task.ErrTaskNotFound
	}

	return nil
}

func (s *Store) Delete(ctx context.Context, id string) (task.Task, error) {
	db, err := s.dbFromCtx(ctx)
	if err != nil {
		return task.Task{}, err
	}

	const query = `
		DELETE FROM tasks
		WHERE id = $1
				RETURNING
					id,
					method,
					url,
					status,
					request_headers,
					http_status_code,
					response_headers,
					length
	`

	item, err := scanTaskRow(db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return task.Task{}, task.ErrTaskNotFound
		}
		return task.Task{}, fmt.Errorf("delete task: %w", err)
	}

	return item, nil
}

func (s *Store) dbFromCtx(ctx context.Context) (DBTX, error) {
	if s.pool == nil {
		return nil, errors.New("postgres pool is nil")
	}

	if ctx != nil {
		if tx, ok := ctx.Value(txContextKey{}).(pgx.Tx); ok {
			return tx, nil
		}
	}

	return s.pool, nil
}

type taskRowScanner interface {
	Scan(dest ...any) error
}

func scanTaskRow(row taskRowScanner) (task.Task, error) {
	var (
		item            task.Task
		status          string
		reqHeadersJSON  []byte
		respHeadersJSON []byte
	)

	if err := row.Scan(
		&item.ID,
		&item.Method,
		&item.URL,
		&status,
		&reqHeadersJSON,
		&item.HTTPStatusCode,
		&respHeadersJSON,
		&item.Length,
	); err != nil {
		return task.Task{}, err
	}

	item.Status = task.Status(status)

	if len(reqHeadersJSON) == 0 {
		item.RequestHeaders = map[string]string{}
	} else if err := json.Unmarshal(reqHeadersJSON, &item.RequestHeaders); err != nil {
		return task.Task{}, fmt.Errorf("unmarshal request headers: %w", err)
	}

	if len(respHeadersJSON) == 0 {
		item.Headers = map[string][]string{}
	} else if err := json.Unmarshal(respHeadersJSON, &item.Headers); err != nil {
		return task.Task{}, fmt.Errorf("unmarshal response headers: %w", err)
	}

	return item, nil
}

func marshalRequestHeaders(headers map[string]string) ([]byte, error) {
	if headers == nil {
		headers = map[string]string{}
	}

	payload, err := json.Marshal(headers)
	if err != nil {
		return nil, fmt.Errorf("marshal request headers: %w", err)
	}

	return payload, nil
}

func marshalResponseHeaders(headers map[string][]string) ([]byte, error) {
	if headers == nil {
		headers = map[string][]string{}
	}

	payload, err := json.Marshal(headers)
	if err != nil {
		return nil, fmt.Errorf("marshal response headers: %w", err)
	}

	return payload, nil
}

package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"todoapp/internal/domain/task"
	service "todoapp/internal/usecase/task"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Store struct {
	db *gorm.DB
}

type txContextKey struct{}

var _ service.Store = (*Store)(nil)
var _ service.TxManager = (*Store)(nil)

func NewStore(db *gorm.DB) *Store {
	return &Store{
		db: db,
	}
}

func (s *Store) Ping(ctx context.Context) error {
	if s.db == nil {
		return errors.New("gorm db is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("get sql db from gorm: %w", err)
	}

	if err = sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}

	return nil
}

func (s *Store) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if s.db == nil {
		return errors.New("gorm db is nil")
	}
	if fn == nil {
		return errors.New("transaction callback is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, txContextKey{}, tx.WithContext(ctx))
		return fn(txCtx)
	}); err != nil {
		return fmt.Errorf("transaction failed: %w", err)
	}

	return nil
}

func (s *Store) Create(ctx context.Context, item task.Task) error {
	db, err := s.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	model := toTaskModel(item)
	model.CreatedAt = time.Now().UTC()

	if err = db.WithContext(ctx).Create(&model).Error; err != nil {
		return fmt.Errorf("insert task: %w", err)
	}

	return nil
}

func (s *Store) GetByID(ctx context.Context, id string) (task.Task, error) {
	db, err := s.dbFromCtx(ctx)
	if err != nil {
		return task.Task{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var model TaskModel
	if err = db.WithContext(ctx).Where("id = ?", id).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return task.Task{}, task.ErrTaskNotFound
		}
		return task.Task{}, fmt.Errorf("select task by id: %w", err)
	}

	return toDomainTask(model), nil
}

func (s *Store) GetAll(ctx context.Context) ([]task.Task, error) {
	db, err := s.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var models []TaskModel
	if err = db.WithContext(ctx).Order("created_at ASC").Order("id ASC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("select all tasks: %w", err)
	}

	return toDomainTasks(models), nil
}

func (s *Store) Update(ctx context.Context, item task.Task) error {
	db, err := s.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	updates := map[string]any{
		"status":           string(item.Status),
		"http_status_code": item.HTTPStatusCode,
		"response_headers": cloneResponseHeaders(item.Headers),
		"length":           item.Length,
		"updated_at":       time.Now().UTC(),
	}

	tx := db.WithContext(ctx).
		Model(&TaskModel{}).
		Where("id = ?", item.ID).
		Updates(updates)
	if tx.Error != nil {
		return fmt.Errorf("update task: %w", tx.Error)
	}
	if tx.RowsAffected == 0 {
		return task.ErrTaskNotFound
	}

	return nil
}

func (s *Store) Delete(ctx context.Context, id string) (task.Task, error) {
	db, err := s.dbFromCtx(ctx)
	if err != nil {
		return task.Task{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var model TaskModel
	tx := db.WithContext(ctx).
		Clauses(clause.Returning{}).
		Where("id = ?", id).
		Delete(&model)
	if tx.Error != nil {
		return task.Task{}, fmt.Errorf("delete task: %w", tx.Error)
	}
	if tx.RowsAffected == 0 {
		return task.Task{}, task.ErrTaskNotFound
	}

	return toDomainTask(model), nil
}

func (s *Store) dbFromCtx(ctx context.Context) (*gorm.DB, error) {
	if s.db == nil {
		return nil, errors.New("gorm db is nil")
	}

	if ctx != nil {
		if tx, ok := ctx.Value(txContextKey{}).(*gorm.DB); ok && tx != nil {
			return tx, nil
		}
	}

	return s.db, nil
}

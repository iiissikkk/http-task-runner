package postgres

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

var ErrNoAppliedMigrations = errors.New("no applied migrations")

func RunMigrations(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return errors.New("gorm db is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if err := db.WithContext(ctx).AutoMigrate(&TaskModel{}); err != nil {
		return fmt.Errorf("auto migrate tasks table: %w", err)
	}

	return nil
}

func RollbackLastMigration(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return errors.New("gorm db is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	migrator := db.WithContext(ctx).Migrator()
	if !migrator.HasTable(&TaskModel{}) {
		return ErrNoAppliedMigrations
	}

	if err := migrator.DropTable(&TaskModel{}); err != nil {
		return fmt.Errorf("drop tasks table: %w", err)
	}

	return nil
}

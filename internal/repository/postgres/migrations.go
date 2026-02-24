package postgres

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

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

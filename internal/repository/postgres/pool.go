package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func NewPool(ctx context.Context, databaseURL string) (*gorm.DB, *sql.DB, error) {
	dsn := strings.TrimSpace(databaseURL)
	if dsn == "" {
		return nil, nil, errors.New("database url is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, nil, fmt.Errorf("open gorm connection: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("get sql db from gorm: %w", err)
	}

	if err = sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, nil, fmt.Errorf("ping postgres: %w", err)
	}

	return db, sqlDB, nil
}

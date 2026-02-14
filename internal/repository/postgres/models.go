package postgres

import (
	"time"
)

type TaskModel struct {
	ID              string
	Method          string
	URL             string
	Status          string
	RequestHeaders  map[string]string
	HTTPStatusCode  int
	ResponseHeaders map[string][]string
	Length          int64
	CreatedAt       time.Time
	UpdatedAt       *time.Time
}

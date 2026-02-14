package postgres

import (
	"time"
)

type TaskModel struct {
	ID             string
	Method         string
	URL            string
	Status         string
	RequestHeaders map[string]string
	CreatedAt      time.Time
	UpdatedAt      *time.Time
}

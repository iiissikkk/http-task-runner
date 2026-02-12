package http

import (
	"encoding/json"
	"errors"
	"time"
)

type TaskDTO struct {
	Title       string
	Description string
}

type CompleteTaskDTO struct {
	Complete bool
}

func (t *TaskDTO) ValidateForCreate() error {
	if t.Title == "" {
		return errors.New("title is required")
	}
	if t.Description == "" {
		return errors.New("description is required")
	}
	return nil
}

type ErrorDTO struct {
	Message string
	Time    time.Time
}

func (e ErrorDTO) ToString() string {
	b, err := json.MarshalIndent(e, "", "    ")
	if err != nil {
		panic(err)
	}
	return string(b)
}

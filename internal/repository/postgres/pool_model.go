package postgres

import (
	"maps"
	"slices"
	"time"
	"todoapp/internal/domain/task"
)

type TaskModel struct {
	ID              string              `gorm:"column:id;type:uuid;primaryKey"`
	Method          string              `gorm:"column:method;type:text;not null"`
	URL             string              `gorm:"column:url;type:text;not null"`
	Status          string              `gorm:"column:status;type:text;not null"`
	RequestHeaders  map[string]string   `gorm:"column:request_headers;type:jsonb;serializer:json;not null"`
	HTTPStatusCode  int                 `gorm:"column:http_status_code;type:integer;not null;default:0"`
	ResponseHeaders map[string][]string `gorm:"column:response_headers;type:jsonb;serializer:json;not null;default:'{}'"`
	Length          int64               `gorm:"column:length;type:bigint;not null;default:0"`
	CreatedAt       time.Time           `gorm:"column:created_at;type:timestamptz;not null;autoCreateTime"`
	UpdatedAt       *time.Time          `gorm:"column:updated_at;type:timestamptz"`
}

func (TaskModel) TableName() string {
	return "tasks"
}

func toTaskModel(item task.Task) TaskModel {
	return TaskModel{
		ID:              item.ID,
		Method:          item.Method,
		URL:             item.URL,
		Status:          string(item.Status),
		RequestHeaders:  cloneRequestHeaders(item.RequestHeaders),
		HTTPStatusCode:  item.HTTPStatusCode,
		ResponseHeaders: cloneResponseHeaders(item.Headers),
		Length:          item.Length,
	}
}

func toDomainTask(model TaskModel) task.Task {
	return task.Task{
		ID:             model.ID,
		Method:         model.Method,
		URL:            model.URL,
		RequestHeaders: cloneRequestHeaders(model.RequestHeaders),
		Status:         task.Status(model.Status),
		HTTPStatusCode: model.HTTPStatusCode,
		Headers:        cloneResponseHeaders(model.ResponseHeaders),
		Length:         model.Length,
	}
}

func toDomainTasks(models []TaskModel) []task.Task {
	out := make([]task.Task, 0, len(models))
	for _, model := range models {
		out = append(out, toDomainTask(model))
	}

	return out
}

func cloneRequestHeaders(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}

	return maps.Clone(input)
}

func cloneResponseHeaders(input map[string][]string) map[string][]string {
	if len(input) == 0 {
		return map[string][]string{}
	}

	out := maps.Clone(input)
	for k, values := range out {
		out[k] = slices.Clone(values)
	}

	return out
}

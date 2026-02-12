package task

import "errors"

var (
	ErrTaskNotFound   = errors.New("task not found")
	ErrMethodRequired = errors.New("method is required")
	ErrURLRequired    = errors.New("url is required")
	ErrInvalidMethod  = errors.New("invalid HTTP method")
	ErrInvalidURL     = errors.New("invalid URL")
)

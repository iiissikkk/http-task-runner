package executor

import (
	"context"
	"net/http"
	"time"

	"todoapp/internal/usecase/task"
)

type HTTPExecutor struct {
	client *http.Client
}

func NewHTTPExecutor(timeout time.Duration) *HTTPExecutor {
	return &HTTPExecutor{client: &http.Client{Timeout: timeout}}
}

func (e *HTTPExecutor) Execute(ctx context.Context, method string, url string, headers map[string]string) (service.ExecuteResult, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return service.ExecuteResult{}, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return service.ExecuteResult{}, err
	}
	defer resp.Body.Close()

	return service.ExecuteResult{
		HTTPStatusCode: resp.StatusCode,
		Headers:        map[string][]string(resp.Header),
		Length:         resp.ContentLength,
	}, nil
}

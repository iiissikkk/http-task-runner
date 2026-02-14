package delivery

type createTaskRequest struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

type createTaskResponse struct {
	ID string `json:"id"`
}

type getTaskResponse struct {
	ID             string              `json:"id"`
	Status         string              `json:"status"`
	HTTPStatusCode int                 `json:"httpStatusCode"`
	Headers        map[string][]string `json:"headers"`
	Length         int64               `json:"length"`
}

type getAllTasksResponse struct {
	Tasks []getTaskResponse `json:"tasks"`
}

type deleteTaskResponse struct {
	ID             string `json:"id"`
	HTTPStatusCode int    `json:"httpStatusCode"`
}

type getHealthStatus struct {
	Status string `json:"status"`
	Port   string `json:"port,omitempty"`
}

type errorResponse struct {
	Error string `json:"error"`
}

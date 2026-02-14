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
	ID             string            `json:"id"`
	Method         string            `json:"method"`
	URL            string            `json:"url"`
	RequestHeaders map[string]string `json:"requestHeaders"`
	Status         string            `json:"status"`
}

type getAllTasksResponse struct {
	Tasks []getTaskResponse `json:"tasks"`
}

type deleteTaskResponse struct {
	ID string `json:"id"`
}

type getHealthStatus struct {
	Status string `json:"status"`
	Port   string `json:"port,omitempty"`
}

type errorResponse struct {
	Error string `json:"error"`
}

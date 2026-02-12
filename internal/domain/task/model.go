package task

type Task struct {
	ID             string
	Method         string
	URL            string
	RequestHeaders map[string]string

	Status         Status
	HTTPStatusCode int
	Headers        map[string][]string
	Length         int64
}

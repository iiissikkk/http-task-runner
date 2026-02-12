package task

type Status string

const (
	StatusNew       Status = "new"
	StatusInProcess Status = "in_process"
	StatusDone      Status = "done"
	StatusError     Status = "error"
)

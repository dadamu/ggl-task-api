package task

// Status is the completion state of a task.
type Status int

const (
	// StatusIncomplete marks a task that is not done yet.
	StatusIncomplete Status = 0
	// StatusCompleted marks a task that is done.
	StatusCompleted Status = 1
)

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	return s == StatusIncomplete || s == StatusCompleted
}

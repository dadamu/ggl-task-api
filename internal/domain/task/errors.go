package task

import (
	"errors"
	"strings"
)

var (
	// ErrNotFound is returned when a task does not exist.
	ErrNotFound = errors.New("task not found")
	// ErrVersionConflict is returned when an update is based on a stale version.
	// Use errors.As with *VersionConflictError to obtain the current task.
	ErrVersionConflict = errors.New("task version conflict")
)

// VersionConflictError reports an optimistic locking failure and carries the
// task as currently stored, so callers can show it to the client.
type VersionConflictError struct {
	Current Task
}

func (e *VersionConflictError) Error() string { return ErrVersionConflict.Error() }

// Is makes errors.Is(err, ErrVersionConflict) match.
func (e *VersionConflictError) Is(target error) bool { return target == ErrVersionConflict }

// FieldError describes why a single field is invalid.
type FieldError struct {
	Field  string
	Reason string
}

// ValidationError reports every invalid field of a task at once.
type ValidationError struct {
	Fields []FieldError
}

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		parts = append(parts, f.Field+": "+f.Reason)
	}
	return "invalid task: " + strings.Join(parts, "; ")
}

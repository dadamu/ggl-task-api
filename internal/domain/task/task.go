// Package task contains the task entity, its invariants and the repository port.
package task

import (
	"strings"
	"time"
	"unicode/utf8"
)

// MaxNameLength is the maximum length of a task name, in characters (runes).
const MaxNameLength = 255

// InitialVersion is the version of a newly created task.
const InitialVersion int64 = 1

// Task is a to-do item.
//
// ID is assigned by the repository on creation. Version is used for optimistic
// locking and is incremented by the repository on every successful update.
type Task struct {
	ID        int64
	Name      string
	Status    Status
	Version   int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// New creates a valid task that has not been persisted yet.
func New(name string, status Status, now time.Time) (Task, error) {
	if err := validate(name, status); err != nil {
		return Task{}, err
	}
	ts := normalizeTime(now)
	return Task{
		Name:      name,
		Status:    status,
		Version:   InitialVersion,
		CreatedAt: ts,
		UpdatedAt: ts,
	}, nil
}

// Update replaces the mutable fields of the task. On error the task is left unchanged.
func (t *Task) Update(name string, status Status, now time.Time) error {
	if err := validate(name, status); err != nil {
		return err
	}
	t.Name = name
	t.Status = status
	t.UpdatedAt = normalizeTime(now)
	return nil
}

func validate(name string, status Status) error {
	var fields []FieldError
	switch {
	case strings.TrimSpace(name) == "":
		fields = append(fields, FieldError{Field: "name", Reason: "must not be blank"})
	case utf8.RuneCountInString(name) > MaxNameLength:
		fields = append(fields, FieldError{Field: "name", Reason: "must be at most 255 characters"})
	}
	if !status.Valid() {
		fields = append(fields, FieldError{Field: "status", Reason: "must be 0 or 1"})
	}
	if len(fields) > 0 {
		return &ValidationError{Fields: fields}
	}
	return nil
}

// normalizeTime converts to UTC (dropping the monotonic reading) with millisecond precision.
func normalizeTime(t time.Time) time.Time {
	return t.UTC().Truncate(time.Millisecond)
}

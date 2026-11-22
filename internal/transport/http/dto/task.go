// Package dto defines the JSON request and response bodies of the HTTP API.
package dto

import (
	"time"

	"github.com/dadamu/ggl-task-api/internal/domain/task"
)

// CreateTaskRequest is the body of POST /tasks.
// Pointer fields distinguish "missing" from the zero value.
type CreateTaskRequest struct {
	Name   *string `json:"name" binding:"required" example:"Buy milk"`
	Status *int    `json:"status" enums:"0,1" example:"0"` // optional, defaults to 0
}

// UpdateTaskRequest is the body of PUT /tasks/{id}. All fields are required
// (full replacement). Version is a precondition: the version the client last read.
type UpdateTaskRequest struct {
	Name    *string `json:"name" binding:"required" example:"Buy oat milk"`
	Status  *int    `json:"status" binding:"required" enums:"0,1" example:"1"`
	Version *int64  `json:"version" binding:"required" example:"1"`
}

// Task is the JSON representation of a task.
type Task struct {
	ID        int64     `json:"id" example:"1"`
	Name      string    `json:"name" example:"Buy milk"`
	Status    int       `json:"status" enums:"0,1" example:"0"`
	Version   int64     `json:"version" example:"1"`
	CreatedAt time.Time `json:"created_at" example:"2026-11-21T08:00:00.123Z"`
	UpdatedAt time.Time `json:"updated_at" example:"2026-11-21T08:00:00.123Z"`
}

// TaskList is the body of GET /tasks: one page of tasks ordered by id.
type TaskList struct {
	Data []Task `json:"data"`
	// NextCursor is the value to pass as "after" for the next page; null on the last page.
	NextCursor *int64 `json:"next_cursor" example:"20"`
}

// FromTask converts a domain task to its JSON representation.
func FromTask(t task.Task) Task {
	return Task{
		ID:        t.ID,
		Name:      t.Name,
		Status:    int(t.Status),
		Version:   t.Version,
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
	}
}

// FromTasks converts one page of domain tasks; Data is never nil, so it encodes as [].
func FromTasks(ts []task.Task, nextCursor *int64) TaskList {
	out := make([]Task, 0, len(ts))
	for _, t := range ts {
		out = append(out, FromTask(t))
	}
	return TaskList{Data: out, NextCursor: nextCursor}
}

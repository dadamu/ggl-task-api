// Package task implements the task use cases. It orchestrates the domain and the
// repository and knows nothing about HTTP.
package task

import (
	"context"
	"time"

	"github.com/dadamu/ggl-task-api/internal/domain/task"
)

// Service implements the task use cases.
type Service struct {
	repo  task.Repository
	clock func() time.Time
}

// NewService returns a Service. clock is the source of current time; pass
// time.Now in production and a fixed clock in tests.
func NewService(repo task.Repository, clock func() time.Time) *Service {
	return &Service{repo: repo, clock: clock}
}

// CreateInput is the input of Create.
type CreateInput struct {
	Name   string
	Status task.Status
}

// UpdateInput is the input of Update. Version is the version the client last
// read; the update succeeds only if it is still current.
type UpdateInput struct {
	ID      int64
	Name    string
	Status  task.Status
	Version int64
}

// Create validates and stores a new task.
// It returns a *task.ValidationError if the input is invalid.
func (s *Service) Create(ctx context.Context, in CreateInput) (task.Task, error) {
	t, err := task.New(in.Name, in.Status, s.clock())
	if err != nil {
		return task.Task{}, err
	}
	return s.repo.Create(ctx, t)
}

// ListInput selects a page of tasks. Limit must be positive; the transport
// layer applies the default and the maximum.
type ListInput struct {
	After int64 // cursor: return tasks with ID greater than After
	Limit int
}

// Page is one page of tasks ordered by ID ascending.
type Page struct {
	Tasks []task.Task
	// NextCursor is the After value for the next page, or nil on the last page.
	NextCursor *int64
}

// List returns one page of tasks. It asks the repository for one extra task
// to tell whether another page exists.
func (s *Service) List(ctx context.Context, in ListInput) (Page, error) {
	tasks, err := s.repo.List(ctx, task.ListQuery{After: in.After, Limit: in.Limit + 1})
	if err != nil {
		return Page{}, err
	}
	if len(tasks) <= in.Limit {
		return Page{Tasks: tasks}, nil
	}
	tasks = tasks[:in.Limit]
	next := tasks[len(tasks)-1].ID
	return Page{Tasks: tasks, NextCursor: &next}, nil
}

// Update replaces the name and status of a task (full replacement).
//
// Errors, in order of precedence: task.ErrNotFound, *task.ValidationError,
// *task.VersionConflictError. The version check is performed atomically by the
// repository, so a concurrent update or delete between Get and Update is detected.
func (s *Service) Update(ctx context.Context, in UpdateInput) (task.Task, error) {
	t, err := s.repo.Get(ctx, in.ID)
	if err != nil {
		return task.Task{}, err
	}
	if err := t.Update(in.Name, in.Status, s.clock()); err != nil {
		return task.Task{}, err
	}
	return s.repo.Update(ctx, t, in.Version)
}

// Delete removes a task. It returns task.ErrNotFound if it does not exist.
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

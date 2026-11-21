// Package memory provides in-memory implementations of domain repositories.
package memory

import (
	"cmp"
	"context"
	"slices"
	"sync"

	"github.com/dadamu/ggl-task-api/internal/domain/task"
)

// TaskRepository is a concurrency-safe, in-memory task.Repository.
//
// Tasks are stored as values and every read returns a copy, so callers can
// never mutate stored state outside the lock. IDs come from a counter that only
// increases, so they are never reused after deletion.
type TaskRepository struct {
	mu     sync.RWMutex
	tasks  map[int64]task.Task
	lastID int64
}

var _ task.Repository = (*TaskRepository)(nil)

// NewTaskRepository returns an empty repository.
func NewTaskRepository() *TaskRepository {
	return &TaskRepository{tasks: make(map[int64]task.Task)}
}

// Create assigns the next ID and stores the task.
//
// ID generation and insertion must stay in one critical section; generating the
// ID outside the lock would let concurrent creates read the same counter value
// and overwrite each other.
func (r *TaskRepository) Create(ctx context.Context, t task.Task) (task.Task, error) {
	if err := ctx.Err(); err != nil {
		return task.Task{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	r.lastID++
	t.ID = r.lastID
	r.tasks[t.ID] = t
	return t, nil
}

// Get returns the task with the given ID, or task.ErrNotFound.
func (r *TaskRepository) Get(ctx context.Context, id int64) (task.Task, error) {
	if err := ctx.Err(); err != nil {
		return task.Task{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.tasks[id]
	if !ok {
		return task.Task{}, task.ErrNotFound
	}
	return t, nil
}

// List returns a consistent snapshot of all tasks ordered by ID ascending.
// The result is never nil.
func (r *TaskRepository) List(ctx context.Context) ([]task.Task, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	out := make([]task.Task, 0, len(r.tasks))
	for _, t := range r.tasks {
		out = append(out, t)
	}
	r.mu.RUnlock()

	// Sorting a private copy does not need the lock.
	slices.SortFunc(out, func(a, b task.Task) int { return cmp.Compare(a.ID, b.ID) })
	return out, nil
}

// Update replaces the stored task if its version equals expectedVersion.
//
// Existence check, version check and write must stay in one critical section.
// Releasing the lock between check and write would be a TOCTOU
// (time-of-check to time-of-use) bug: a concurrent Delete or Update could slip
// into the gap, so a deleted task would be resurrected or a stale write accepted.
// Do not "optimize" this into an RLock check followed by a separate Lock write.
func (r *TaskRepository) Update(ctx context.Context, t task.Task, expectedVersion int64) (task.Task, error) {
	if err := ctx.Err(); err != nil {
		return task.Task{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	cur, ok := r.tasks[t.ID]
	if !ok {
		return task.Task{}, task.ErrNotFound
	}
	if cur.Version != expectedVersion {
		return task.Task{}, &task.VersionConflictError{Current: cur}
	}
	t.Version = cur.Version + 1
	r.tasks[t.ID] = t
	return t, nil
}

// Delete removes the task with the given ID, or returns task.ErrNotFound.
func (r *TaskRepository) Delete(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.tasks[id]; !ok {
		return task.ErrNotFound
	}
	delete(r.tasks, id)
	return nil
}

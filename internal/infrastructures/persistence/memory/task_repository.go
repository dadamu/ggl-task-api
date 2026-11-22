// Package memory provides in-memory implementations of domain repositories.
package memory

import (
	"context"
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

// List returns up to q.Limit tasks with ID greater than q.After, ordered by ID,
// from a consistent snapshot. The result is never nil.
//
// IDs are assigned by an increasing counter and never reused (ADR-0002), so
// walking IDs upward from q.After yields tasks already in order: no full scan
// and no sort. Cost is O(limit + deleted IDs skipped), independent of the total
// number of tasks in the common case.
func (r *TaskRepository) List(ctx context.Context, q task.ListQuery) ([]task.Task, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]task.Task, 0, min(q.Limit, len(r.tasks)))
	for id := max(q.After, 0) + 1; id <= r.lastID && len(out) < q.Limit; id++ {
		if t, ok := r.tasks[id]; ok {
			out = append(out, t)
		}
	}
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

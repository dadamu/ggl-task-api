package task

import "context"

// Repository persists tasks. Implementations must be safe for concurrent use and
// must return copies, never references to internal state.
type Repository interface {
	// Create assigns a new, never-reused ID and stores the task.
	Create(ctx context.Context, t Task) (Task, error)

	// Get returns the task with the given ID, or ErrNotFound.
	Get(ctx context.Context, id int64) (Task, error)

	// List returns all tasks ordered by ID ascending. It never returns nil.
	List(ctx context.Context) ([]Task, error)

	// Update atomically replaces the stored task with t if the stored version
	// equals expectedVersion (a precondition, not a value to set). On success the
	// stored version is incremented and the stored result is returned; t.Version
	// is ignored. It returns ErrNotFound if the task does not exist, or a
	// *VersionConflictError (matching ErrVersionConflict) if the version is stale.
	Update(ctx context.Context, t Task, expectedVersion int64) (Task, error)

	// Delete removes the task with the given ID, or returns ErrNotFound.
	Delete(ctx context.Context, id int64) error
}

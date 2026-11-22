package task_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	app "github.com/dadamu/ggl-task-api/internal/application/task"
	"github.com/dadamu/ggl-task-api/internal/domain/task"
	"github.com/dadamu/ggl-task-api/internal/infrastructures/persistence/memory"
)

var t0 = time.Date(2026, 11, 21, 8, 0, 0, 0, time.UTC)

// fakeClock returns a fixed time that tests can advance.
type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time          { return c.now }
func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func newService(t *testing.T) (*app.Service, *memory.TaskRepository, *fakeClock) {
	t.Helper()
	repo := memory.NewTaskRepository()
	clk := &fakeClock{now: t0}
	return app.NewService(repo, clk.Now), repo, clk
}

func mustCreate(t *testing.T, svc *app.Service, name string) task.Task {
	t.Helper()
	created, err := svc.Create(t.Context(), app.CreateInput{Name: name})
	require.NoError(t, err)
	return created
}

func TestCreate(t *testing.T) {
	svc, _, _ := newService(t)

	got, err := svc.Create(t.Context(), app.CreateInput{Name: "Buy milk", Status: task.StatusCompleted})

	require.NoError(t, err)
	require.Equal(t, task.Task{
		ID:        1,
		Name:      "Buy milk",
		Status:    task.StatusCompleted,
		Version:   task.InitialVersion,
		CreatedAt: t0,
		UpdatedAt: t0,
	}, got)
}

func TestCreateInvalidIsNotStored(t *testing.T) {
	svc, repo, _ := newService(t)

	_, err := svc.Create(t.Context(), app.CreateInput{Name: "   ", Status: 2})

	var verr *task.ValidationError
	require.ErrorAs(t, err, &verr)
	require.Len(t, verr.Fields, 2)
	listed, err := repo.List(t.Context(), task.ListQuery{Limit: 100})
	require.NoError(t, err)
	require.Empty(t, listed)
}

func TestList(t *testing.T) {
	cursor := func(id int64) *int64 { return &id }

	tests := []struct {
		name     string
		in       app.ListInput
		wantIDs  []int64
		wantNext *int64
	}{
		{"first page", app.ListInput{Limit: 2}, []int64{1, 2}, cursor(2)},
		{"middle page", app.ListInput{After: 2, Limit: 2}, []int64{3, 4}, cursor(4)},
		{"last partial page", app.ListInput{After: 4, Limit: 2}, []int64{5}, nil},
		{"exact fit has no next page", app.ListInput{Limit: 5}, []int64{1, 2, 3, 4, 5}, nil},
		{"limit larger than total", app.ListInput{Limit: 10}, []int64{1, 2, 3, 4, 5}, nil},
		{"after the last task", app.ListInput{After: 5, Limit: 2}, []int64{}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, _ := newService(t)
			for _, name := range []string{"a", "b", "c", "d", "e"} {
				mustCreate(t, svc, name)
			}

			got, err := svc.List(t.Context(), tt.in)

			require.NoError(t, err)
			require.Equal(t, tt.wantIDs, ids(got.Tasks))
			require.Equal(t, tt.wantNext, got.NextCursor)
		})
	}
}

func TestListCursorSurvivesDeletion(t *testing.T) {
	svc, _, _ := newService(t)
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		mustCreate(t, svc, name)
	}
	first, err := svc.List(t.Context(), app.ListInput{Limit: 2})
	require.NoError(t, err)
	require.NoError(t, svc.Delete(t.Context(), *first.NextCursor)) // delete the cursor task itself
	require.NoError(t, svc.Delete(t.Context(), 3))

	second, err := svc.List(t.Context(), app.ListInput{After: *first.NextCursor, Limit: 2})

	require.NoError(t, err)
	require.Equal(t, []int64{4, 5}, ids(second.Tasks), "no task skipped or repeated")
}

func ids(tasks []task.Task) []int64 {
	out := make([]int64, len(tasks))
	for i, tk := range tasks {
		out[i] = tk.ID
	}
	return out
}

func TestUpdate(t *testing.T) {
	svc, _, clk := newService(t)
	created := mustCreate(t, svc, "a")
	clk.Advance(time.Hour)

	got, err := svc.Update(t.Context(), app.UpdateInput{
		ID: created.ID, Name: "b", Status: task.StatusCompleted, Version: created.Version,
	})

	require.NoError(t, err)
	require.Equal(t, task.Task{
		ID:        created.ID,
		Name:      "b",
		Status:    task.StatusCompleted,
		Version:   created.Version + 1,
		CreatedAt: t0,                // preserved
		UpdatedAt: t0.Add(time.Hour), // from the clock
	}, got)
}

func TestUpdateNotFound(t *testing.T) {
	svc, _, _ := newService(t)

	// Invalid input too: not found takes precedence over validation.
	_, err := svc.Update(t.Context(), app.UpdateInput{ID: 999, Name: "", Status: 2, Version: 1})

	require.ErrorIs(t, err, task.ErrNotFound)
}

func TestUpdateInvalidLeavesTaskUnchanged(t *testing.T) {
	svc, repo, _ := newService(t)
	created := mustCreate(t, svc, "a")

	// Stale version too: validation takes precedence over the version check.
	_, err := svc.Update(t.Context(), app.UpdateInput{ID: created.ID, Name: " ", Status: 0, Version: 99})

	var verr *task.ValidationError
	require.ErrorAs(t, err, &verr)
	stored, err := repo.Get(t.Context(), created.ID)
	require.NoError(t, err)
	require.Equal(t, created, stored)
}

func TestUpdateStaleVersion(t *testing.T) {
	svc, _, _ := newService(t)
	created := mustCreate(t, svc, "a")
	current, err := svc.Update(t.Context(), app.UpdateInput{ID: created.ID, Name: "b", Version: created.Version})
	require.NoError(t, err)

	_, err = svc.Update(t.Context(), app.UpdateInput{ID: created.ID, Name: "c", Version: created.Version})

	var cerr *task.VersionConflictError
	require.ErrorAs(t, err, &cerr)
	require.Equal(t, current, cerr.Current)
}

func TestDelete(t *testing.T) {
	svc, repo, _ := newService(t)
	created := mustCreate(t, svc, "a")

	require.NoError(t, svc.Delete(t.Context(), created.ID))
	require.ErrorIs(t, svc.Delete(t.Context(), created.ID), task.ErrNotFound)

	_, err := repo.Get(t.Context(), created.ID)
	require.ErrorIs(t, err, task.ErrNotFound)
}

// --- Unexpected repository errors ---

var errBoom = errors.New("boom")

// failingRepo embeds a real repository and fails the configured methods.
type failingRepo struct {
	task.Repository
	failCreate, failGet, failList, failUpdate, failDelete bool
}

func (r failingRepo) Create(ctx context.Context, t task.Task) (task.Task, error) {
	if r.failCreate {
		return task.Task{}, errBoom
	}
	return r.Repository.Create(ctx, t)
}

func (r failingRepo) Get(ctx context.Context, id int64) (task.Task, error) {
	if r.failGet {
		return task.Task{}, errBoom
	}
	return r.Repository.Get(ctx, id)
}

func (r failingRepo) List(ctx context.Context, q task.ListQuery) ([]task.Task, error) {
	if r.failList {
		return nil, errBoom
	}
	return r.Repository.List(ctx, q)
}

func (r failingRepo) Update(ctx context.Context, t task.Task, v int64) (task.Task, error) {
	if r.failUpdate {
		return task.Task{}, errBoom
	}
	return r.Repository.Update(ctx, t, v)
}

func (r failingRepo) Delete(ctx context.Context, id int64) error {
	if r.failDelete {
		return errBoom
	}
	return r.Repository.Delete(ctx, id)
}

func TestRepositoryErrorsPropagate(t *testing.T) {
	newFailing := func(t *testing.T, f failingRepo) (*app.Service, task.Task) {
		t.Helper()
		f.Repository = memory.NewTaskRepository()
		created, err := f.Repository.Create(t.Context(), task.Task{Name: "a", Version: 1})
		require.NoError(t, err)
		return app.NewService(f, func() time.Time { return t0 }), created
	}
	update := func(created task.Task) app.UpdateInput {
		return app.UpdateInput{ID: created.ID, Name: "b", Version: created.Version}
	}

	t.Run("create", func(t *testing.T) {
		svc, _ := newFailing(t, failingRepo{failCreate: true})
		_, err := svc.Create(t.Context(), app.CreateInput{Name: "x"})
		require.ErrorIs(t, err, errBoom)
	})
	t.Run("list", func(t *testing.T) {
		svc, _ := newFailing(t, failingRepo{failList: true})
		_, err := svc.List(t.Context(), app.ListInput{Limit: 20})
		require.ErrorIs(t, err, errBoom)
	})
	t.Run("update: get fails", func(t *testing.T) {
		svc, created := newFailing(t, failingRepo{failGet: true})
		_, err := svc.Update(t.Context(), update(created))
		require.ErrorIs(t, err, errBoom)
	})
	t.Run("update: write fails", func(t *testing.T) {
		svc, created := newFailing(t, failingRepo{failUpdate: true})
		_, err := svc.Update(t.Context(), update(created))
		require.ErrorIs(t, err, errBoom)
	})
	t.Run("delete", func(t *testing.T) {
		svc, created := newFailing(t, failingRepo{failDelete: true})
		require.ErrorIs(t, svc.Delete(t.Context(), created.ID), errBoom)
	})
}

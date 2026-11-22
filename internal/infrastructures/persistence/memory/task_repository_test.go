package memory_test

import (
	"context"
	"math"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dadamu/ggl-task-api/internal/domain/task"
	"github.com/dadamu/ggl-task-api/internal/infrastructures/persistence/memory"
)

var now = time.Date(2026, 11, 21, 8, 0, 0, 0, time.UTC)

// all lists every task in one page.
var all = task.ListQuery{Limit: math.MaxInt}

func newTask(t *testing.T, name string) task.Task {
	t.Helper()
	tk, err := task.New(name, task.StatusIncomplete, now)
	require.NoError(t, err)
	return tk
}

func mustCreate(t *testing.T, repo *memory.TaskRepository, name string) task.Task {
	t.Helper()
	created, err := repo.Create(t.Context(), newTask(t, name))
	require.NoError(t, err)
	return created
}

// seed creates n tasks named "0".."n-1" with IDs 1..n.
func seed(t *testing.T, repo *memory.TaskRepository, n int) {
	t.Helper()
	for i := range n {
		mustCreate(t, repo, strconv.Itoa(i))
	}
}

// requireStored asserts the repository currently stores exactly want.
func requireStored(t *testing.T, repo *memory.TaskRepository, want task.Task) {
	t.Helper()
	got, err := repo.Get(t.Context(), want.ID)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func ids(tasks []task.Task) []int64 {
	out := make([]int64, len(tasks))
	for i, tk := range tasks {
		out[i] = tk.ID
	}
	return out
}

// --- Scenarios ---

func TestCreateAssignsSequentialIDs(t *testing.T) {
	repo := memory.NewTaskRepository()

	first := mustCreate(t, repo, "a")
	second := mustCreate(t, repo, "b")

	require.Equal(t, int64(1), first.ID)
	require.Equal(t, int64(2), second.ID)
	require.Equal(t, task.InitialVersion, first.Version)
}

func TestIDsAreNeverReused(t *testing.T) {
	repo := memory.NewTaskRepository()
	seed(t, repo, 2)
	require.NoError(t, repo.Delete(t.Context(), 2))

	third := mustCreate(t, repo, "c")

	require.Equal(t, int64(3), third.ID)
}

func TestReturnedValuesAreCopies(t *testing.T) {
	repo := memory.NewTaskRepository()
	created := mustCreate(t, repo, "original")

	listed, err := repo.List(t.Context(), all)
	require.NoError(t, err)
	listed[0].Name = "mutated"

	got, err := repo.Get(t.Context(), created.ID)
	require.NoError(t, err)
	got.Name = "mutated too"

	stored, err := repo.Get(t.Context(), created.ID)
	require.NoError(t, err)
	require.Equal(t, "original", stored.Name)
}

// --- Table-driven ---

func TestGet(t *testing.T) {
	tests := []struct {
		name     string
		id       int64
		wantName string
		wantErr  error
	}{
		{name: "existing", id: 2, wantName: "1"},
		{name: "not found", id: 999, wantErr: task.ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := memory.NewTaskRepository()
			seed(t, repo, 3)

			got, err := repo.Get(t.Context(), tt.id)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.id, got.ID)
			require.Equal(t, tt.wantName, got.Name)
		})
	}
}

func TestList(t *testing.T) {
	tests := []struct {
		name    string
		seed    int
		deletes []int64
		wantIDs []int64
	}{
		{name: "empty is not nil", seed: 0, wantIDs: []int64{}},
		{name: "ordered by id", seed: 5, wantIDs: []int64{1, 2, 3, 4, 5}},
		{name: "ordered after deletes", seed: 5, deletes: []int64{1, 3}, wantIDs: []int64{2, 4, 5}},
		{name: "empty after deleting all", seed: 2, deletes: []int64{1, 2}, wantIDs: []int64{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := memory.NewTaskRepository()
			seed(t, repo, tt.seed)
			for _, id := range tt.deletes {
				require.NoError(t, repo.Delete(t.Context(), id))
			}

			got, err := repo.List(t.Context(), all)

			require.NoError(t, err)
			require.NotNil(t, got)
			require.Equal(t, tt.wantIDs, ids(got))
		})
	}
}

func TestListQuery(t *testing.T) {
	tests := []struct {
		name    string
		q       task.ListQuery
		wantIDs []int64
	}{
		{"limit from start", task.ListQuery{Limit: 2}, []int64{1, 2}},
		{"after excludes the cursor id", task.ListQuery{After: 2, Limit: 2}, []int64{4, 5}}, // 3 deleted
		{"after a deleted id", task.ListQuery{After: 3, Limit: 10}, []int64{4, 5}},
		{"after the last id", task.ListQuery{After: 5, Limit: 10}, []int64{}},
		{"limit larger than remaining", task.ListQuery{After: 1, Limit: 10}, []int64{2, 4, 5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := memory.NewTaskRepository()
			seed(t, repo, 5)
			require.NoError(t, repo.Delete(t.Context(), 3))

			got, err := repo.List(t.Context(), tt.q)

			require.NoError(t, err)
			require.Equal(t, tt.wantIDs, ids(got))
		})
	}
}

func TestUpdate(t *testing.T) {
	repo := memory.NewTaskRepository()
	created := mustCreate(t, repo, "a")

	input := created
	require.NoError(t, input.Update("b", task.StatusCompleted, now.Add(time.Minute)))
	input.Version = 99 // ignored: the repository sets the version

	got, err := repo.Update(t.Context(), input, created.Version)
	require.NoError(t, err)

	want := input
	want.Version = created.Version + 1
	require.Equal(t, want, got)

	stored, err := repo.Get(t.Context(), created.ID)
	require.NoError(t, err)
	require.Equal(t, want, stored)
}

func TestUpdateStaleVersion(t *testing.T) {
	repo := memory.NewTaskRepository()
	created := mustCreate(t, repo, "a")
	current, err := repo.Update(t.Context(), created, created.Version) // now version 2
	require.NoError(t, err)

	_, err = repo.Update(t.Context(), created, created.Version) // stale: expects 1

	var cerr *task.VersionConflictError
	require.ErrorAs(t, err, &cerr)
	require.Equal(t, current, cerr.Current, "conflict carries the stored task")
	requireStored(t, repo, current)
}

func TestUpdateFutureVersion(t *testing.T) {
	repo := memory.NewTaskRepository()
	created := mustCreate(t, repo, "a")

	_, err := repo.Update(t.Context(), created, created.Version+4)

	require.ErrorIs(t, err, task.ErrVersionConflict, "versions must match exactly")
	requireStored(t, repo, created)
}

func TestUpdateNotFound(t *testing.T) {
	repo := memory.NewTaskRepository()

	_, err := repo.Update(t.Context(), task.Task{ID: 1}, 1)

	require.ErrorIs(t, err, task.ErrNotFound)
}

func TestDelete(t *testing.T) {
	tests := []struct {
		name    string
		id      int64
		wantErr error
		wantIDs []int64
	}{
		{name: "existing", id: 2, wantIDs: []int64{1, 3}},
		{name: "not found", id: 999, wantErr: task.ErrNotFound, wantIDs: []int64{1, 2, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := memory.NewTaskRepository()
			seed(t, repo, 3)

			err := repo.Delete(t.Context(), tt.id)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				_, getErr := repo.Get(t.Context(), tt.id)
				require.ErrorIs(t, getErr, task.ErrNotFound)
			}
			got, err := repo.List(t.Context(), all)
			require.NoError(t, err)
			require.Equal(t, tt.wantIDs, ids(got))
		})
	}
}

func TestCanceledContext(t *testing.T) {
	repo := memory.NewTaskRepository()
	created := mustCreate(t, repo, "a")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := repo.Create(ctx, created)
	require.ErrorIs(t, err, context.Canceled)
	_, err = repo.Get(ctx, created.ID)
	require.ErrorIs(t, err, context.Canceled)
	_, err = repo.List(ctx, all)
	require.ErrorIs(t, err, context.Canceled)
	_, err = repo.Update(ctx, created, created.Version)
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, repo.Delete(ctx, created.ID), context.Canceled)

	// Canceled calls must not change state.
	requireStored(t, repo, created)
	listed, err := repo.List(t.Context(), all)
	require.NoError(t, err)
	require.Len(t, listed, 1)
}

// --- Concurrency (run with -race) ---

// runConcurrently starts n goroutines that begin at the same moment and waits for all.
func runConcurrently(n int, fn func(i int)) {
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			<-start
			fn(i)
		})
	}
	close(start)
	wg.Wait()
}

// Guards against generating IDs outside the write lock.
func TestConcurrentCreatesHaveUniqueIDs(t *testing.T) {
	const n = 200
	repo := memory.NewTaskRepository()
	created := make([]task.Task, n)
	errs := make([]error, n)

	runConcurrently(n, func(i int) {
		created[i], errs[i] = repo.Create(t.Context(), newTask(t, strconv.Itoa(i)))
	})

	seen := make(map[int64]bool, n)
	for i := range n {
		require.NoError(t, errs[i])
		require.False(t, seen[created[i].ID], "duplicate id %d", created[i].ID)
		seen[created[i].ID] = true
	}
	listed, err := repo.List(t.Context(), all)
	require.NoError(t, err)
	require.Len(t, listed, n)
}

// Guards against a TOCTOU Update: if the version check and the write were in
// separate critical sections, several updates could pass the check before any writes.
func TestConcurrentUpdatesSameVersionExactlyOneWins(t *testing.T) {
	const n = 50
	repo := memory.NewTaskRepository()
	created := mustCreate(t, repo, "a")
	errs := make([]error, n)

	runConcurrently(n, func(i int) {
		changed := created
		changed.Name = strconv.Itoa(i)
		_, errs[i] = repo.Update(t.Context(), changed, created.Version)
	})

	wins := 0
	for _, err := range errs {
		if err == nil {
			wins++
			continue
		}
		require.ErrorIs(t, err, task.ErrVersionConflict)
	}
	require.Equal(t, 1, wins)

	stored, err := repo.Get(t.Context(), created.ID)
	require.NoError(t, err)
	require.Equal(t, created.Version+1, stored.Version)
}

// Guards against a TOCTOU Update: if the existence check and the write were in
// separate critical sections, a Delete in between would be undone by the write.
func TestConcurrentUpdateAndDeleteNeverResurrects(t *testing.T) {
	repo := memory.NewTaskRepository()

	for range 200 {
		created := mustCreate(t, repo, "a")
		var updateErr, deleteErr error

		runConcurrently(2, func(i int) {
			if i == 0 {
				deleteErr = repo.Delete(t.Context(), created.ID)
				return
			}
			_, updateErr = repo.Update(t.Context(), created, created.Version)
		})

		require.NoError(t, deleteErr)
		if updateErr != nil {
			require.ErrorIs(t, updateErr, task.ErrNotFound)
		}
		_, err := repo.Get(t.Context(), created.ID)
		require.ErrorIs(t, err, task.ErrNotFound, "deleted task must stay deleted")
	}
}

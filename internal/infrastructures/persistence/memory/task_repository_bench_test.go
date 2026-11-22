package memory_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/dadamu/ggl-task-api/internal/domain/task"
	"github.com/dadamu/ggl-task-api/internal/infrastructures/persistence/memory"
)

// benchSizes are the task counts used to see how List scales.
var benchSizes = []int{100, 1_000, 10_000, 100_000}

func seededRepo(b *testing.B, n int) *memory.TaskRepository {
	b.Helper()
	repo := memory.NewTaskRepository()
	now := time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)
	for i := range n {
		t, err := task.New("task "+strconv.Itoa(i), task.StatusIncomplete, now)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := repo.Create(context.Background(), t); err != nil {
			b.Fatal(err)
		}
	}
	return repo
}

// BenchmarkList measures the repository part of GET /tasks. Every call scans
// the whole map, so even a 20-item page costs O(n).
func BenchmarkList(b *testing.B) {
	for _, n := range benchSizes {
		queries := map[string]task.ListQuery{
			"all":         {Limit: n},
			"page20_head": {Limit: 20},
			"page20_mid":  {After: int64(n / 2), Limit: 20},
		}
		for _, name := range []string{"all", "page20_head", "page20_mid"} {
			b.Run("n="+strconv.Itoa(n)+"/"+name, func(b *testing.B) {
				repo := seededRepo(b, n)
				ctx := context.Background()
				b.ReportAllocs()
				for b.Loop() {
					if _, err := repo.List(ctx, queries[name]); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

// BenchmarkListSparse is the worst case for walking IDs: most IDs were deleted,
// so the first page has to skip over them.
func BenchmarkListSparse(b *testing.B) {
	const n, kept = 100_000, 20
	repo := seededRepo(b, n)
	ctx := context.Background()
	for id := int64(1); id <= n-kept; id++ {
		if err := repo.Delete(ctx, id); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		got, err := repo.List(ctx, task.ListQuery{Limit: 20})
		if err != nil || len(got) != kept {
			b.Fatalf("got %d tasks, err %v", len(got), err)
		}
	}
}

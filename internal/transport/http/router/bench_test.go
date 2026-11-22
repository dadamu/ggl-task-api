package router_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	app "github.com/dadamu/ggl-task-api/internal/application/task"
	"github.com/dadamu/ggl-task-api/internal/domain/task"
	"github.com/dadamu/ggl-task-api/internal/infrastructures/persistence/memory"
	"github.com/dadamu/ggl-task-api/internal/transport/http/router"
)

// BenchmarkListTasksHTTP measures GET /tasks end to end in-process (router,
// handler, application, repository, JSON encoding; no network). The
// "resp_bytes" metric is the response size.
func BenchmarkListTasksHTTP(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	for _, n := range []int{100, 1_000, 10_000, 100_000} {
		b.Run("n="+strconv.Itoa(n), func(b *testing.B) {
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
			// gin.Logger reads DefaultWriter when the router is built; discard
			// access logs so terminal output does not dominate the measurement.
			gin.DefaultWriter = io.Discard
			r := router.New(app.NewService(repo, time.Now), router.Options{})

			var size int
			b.ReportAllocs()
			for b.Loop() {
				rec := httptest.NewRecorder()
				r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tasks", http.NoBody))
				if rec.Code != http.StatusOK {
					b.Fatalf("status %d", rec.Code)
				}
				size = rec.Body.Len()
			}
			b.ReportMetric(float64(size), "resp_bytes")
		})
	}
}

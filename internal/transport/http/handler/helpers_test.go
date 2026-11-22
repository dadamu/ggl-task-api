package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	app "github.com/dadamu/ggl-task-api/internal/application/task"
	"github.com/dadamu/ggl-task-api/internal/domain/task"
	"github.com/dadamu/ggl-task-api/internal/infrastructures/persistence/memory"
	"github.com/dadamu/ggl-task-api/internal/transport/http/apierror"
	"github.com/dadamu/ggl-task-api/internal/transport/http/dto"
	"github.com/dadamu/ggl-task-api/internal/transport/http/handler"
)

// t0 has sub-millisecond precision to verify truncation to milliseconds.
var t0 = time.Date(2026, 11, 21, 8, 0, 0, 123456789, time.UTC)

func init() { gin.SetMode(gin.TestMode) }

func newServer(t *testing.T) http.Handler {
	t.Helper()
	return newServerWithRepo(t, memory.NewTaskRepository())
}

// newServerWithRepo mounts only the task handler on a bare engine: these tests
// cover handler behavior; routing (404/405, middleware) is tested in router.
func newServerWithRepo(t *testing.T, repo task.Repository) http.Handler {
	t.Helper()
	h := handler.NewTask(app.NewService(repo, func() time.Time { return t0 }))
	r := gin.New()
	r.GET("/tasks", h.List)
	r.POST("/tasks", h.Create)
	r.PUT("/tasks/:id", h.Update)
	r.DELETE("/tasks/:id", h.Delete)
	return r
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// decode asserts the response is JSON and decodes its body.
func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	require.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))
	var v T
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &v), "body: %s", rec.Body.String())
	return v
}

func requireError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) apierror.Body {
	t.Helper()
	require.Equal(t, status, rec.Code, "body: %s", rec.Body.String())
	body := decode[apierror.Response](t, rec).Error
	require.Equal(t, code, body.Code)
	require.NotEmpty(t, body.Message)
	return body
}

func create(t *testing.T, h http.Handler, body string) dto.Task {
	t.Helper()
	rec := do(t, h, http.MethodPost, "/tasks", body)
	require.Equal(t, http.StatusCreated, rec.Code, "body: %s", rec.Body.String())
	return decode[dto.Task](t, rec)
}

package router_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	app "github.com/dadamu/ggl-task-api/internal/application/task"
	"github.com/dadamu/ggl-task-api/internal/infrastructures/persistence/memory"
	"github.com/dadamu/ggl-task-api/internal/transport/http/apierror"
	"github.com/dadamu/ggl-task-api/internal/transport/http/router"
)

// Router tests cover routing only: which paths exist, router-level errors and
// trailing-slash behavior. Endpoint behavior is tested in the handler package.

func init() { gin.SetMode(gin.TestMode) }

func newRouter() http.Handler {
	return router.New(app.NewService(memory.NewTaskRepository(), time.Now))
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

func requireRouterError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	require.Equal(t, status, rec.Code, "body: %s", rec.Body.String())
	var resp apierror.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, code, resp.Error.Code)
}

func TestRoutesAreWired(t *testing.T) {
	r := newRouter()

	// Executed in order against one router so PUT/DELETE target the created task.
	steps := []struct {
		method, path, body string
		want               int
	}{
		{http.MethodGet, "/healthz", "", http.StatusOK},
		{http.MethodGet, "/swagger/index.html", "", http.StatusOK},
		{http.MethodPost, "/tasks", `{"name":"a"}`, http.StatusCreated},
		{http.MethodGet, "/tasks", "", http.StatusOK},
		{http.MethodPut, "/tasks/1", `{"name":"b","status":1,"version":1}`, http.StatusOK},
		{http.MethodDelete, "/tasks/1", "", http.StatusNoContent},
	}
	for _, s := range steps {
		rec := do(t, r, s.method, s.path, s.body)
		require.Equal(t, s.want, rec.Code, "%s %s: %s", s.method, s.path, rec.Body.String())
	}
}

// Verifies RequireJSON is attached to body routes only; its behavior is tested
// in the middleware package.
func TestJSONContentTypeRequiredOnBodyRoutes(t *testing.T) {
	r := newRouter()
	create := do(t, r, http.MethodPost, "/tasks", `{"name":"a"}`)
	require.Equal(t, http.StatusCreated, create.Code)

	tests := []struct {
		method, path, body string
		want               int
	}{
		{http.MethodPost, "/tasks", `{"name":"a"}`, http.StatusUnsupportedMediaType},
		{http.MethodPut, "/tasks/1", `{"name":"b","status":0,"version":1}`, http.StatusUnsupportedMediaType},
		{http.MethodGet, "/tasks", "", http.StatusOK},
		{http.MethodDelete, "/tasks/1", "", http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "text/plain")
			rec := httptest.NewRecorder()

			r.ServeHTTP(rec, req)

			require.Equal(t, tt.want, rec.Code, "body: %s", rec.Body.String())
		})
	}
}

// Verifies BodyLimit is attached with the configured limit; its behavior is
// tested in the middleware package.
func TestBodyLimitIsApplied(t *testing.T) {
	r := newRouter()
	atLimit := `{"name":"` + strings.Repeat("a", router.MaxBodyBytes-len(`{"name":""}`)) + `"}`
	overLimit := atLimit + " "

	// At the limit the body is accepted by BodyLimit (and then rejected by domain validation).
	require.Equal(t, http.StatusBadRequest, do(t, r, http.MethodPost, "/tasks", atLimit).Code)
	requireRouterError(t, do(t, r, http.MethodPost, "/tasks", overLimit), http.StatusRequestEntityTooLarge, apierror.CodePayloadTooLarge)
}

// Verifies Recovery is attached; its behavior is tested in the middleware package.
func TestRecoveryIsApplied(t *testing.T) {
	gin.DefaultErrorWriter = io.Discard // keep the expected stack trace out of test output
	r := router.New(app.NewService(memory.NewTaskRepository(), time.Now))
	r.GET("/panic", func(*gin.Context) { panic("boom") })

	requireRouterError(t, do(t, r, http.MethodGet, "/panic", ""), http.StatusInternalServerError, apierror.CodeInternal)
}

func TestRouteNotFound(t *testing.T) {
	requireRouterError(t, do(t, newRouter(), http.MethodGet, "/nope", ""), http.StatusNotFound, apierror.CodeRouteNotFound)
}

func TestTrailingSlashIsNotRedirected(t *testing.T) {
	requireRouterError(t, do(t, newRouter(), http.MethodGet, "/tasks/", ""), http.StatusNotFound, apierror.CodeRouteNotFound)
}

func TestMethodNotAllowed(t *testing.T) {
	tests := []struct {
		method, path, wantAllow string
	}{
		{http.MethodPatch, "/tasks/1", "PUT, DELETE"},
		{http.MethodPost, "/tasks/1", "PUT, DELETE"},
		{http.MethodDelete, "/tasks", "GET, POST"},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rec := do(t, newRouter(), tt.method, tt.path, "")

			requireRouterError(t, rec, http.StatusMethodNotAllowed, apierror.CodeMethodNotAllowed)
			require.Equal(t, tt.wantAllow, rec.Header().Get("Allow"))
		})
	}
}

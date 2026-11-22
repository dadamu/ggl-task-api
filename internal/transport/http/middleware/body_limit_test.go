package middleware_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/dadamu/ggl-task-api/internal/transport/http/apierror"
	"github.com/dadamu/ggl-task-api/internal/transport/http/middleware"
)

const limit = 10

// readResult records what the handler behind BodyLimit observed.
type readResult struct {
	ran  bool
	body string
	err  error
}

// newLimited returns an engine whose handler reads the whole body into res.
func newLimited(res *readResult) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/", middleware.BodyLimit(limit), func(c *gin.Context) {
		b, err := io.ReadAll(c.Request.Body)
		*res = readResult{ran: true, body: string(b), err: err}
		c.Status(http.StatusNoContent)
	})
	return r
}

func post(t *testing.T, r http.Handler, body string, contentLength int64) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader(body))
	req.ContentLength = contentLength
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestBodyLimitAllowsBodiesUpToLimit(t *testing.T) {
	for _, body := range []string{"", "short", strings.Repeat("a", limit)} {
		var res readResult
		rec := post(t, newLimited(&res), body, int64(len(body)))

		require.Equal(t, http.StatusNoContent, rec.Code)
		require.NoError(t, res.err)
		require.Equal(t, body, res.body)
	}
}

func TestBodyLimitRejectsDeclaredLengthBeforeReading(t *testing.T) {
	var res readResult
	body := strings.Repeat("a", limit+1)

	rec := post(t, newLimited(&res), body, int64(len(body)))

	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	require.False(t, res.ran, "handler must not run")
	var resp apierror.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, apierror.CodePayloadTooLarge, resp.Error.Code)
}

func TestBodyLimitCapsUndeclaredLength(t *testing.T) {
	var res readResult

	// -1: unknown length, e.g. chunked transfer encoding.
	post(t, newLimited(&res), strings.Repeat("a", limit+1), -1)

	require.True(t, res.ran)
	var maxErr *http.MaxBytesError
	require.ErrorAs(t, res.err, &maxErr, "reading past the limit must fail")
	require.Equal(t, int64(limit), maxErr.Limit)
	require.Len(t, res.body, limit)
}

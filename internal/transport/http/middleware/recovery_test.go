package middleware_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/dadamu/ggl-task-api/internal/transport/http/apierror"
	"github.com/dadamu/ggl-task-api/internal/transport/http/middleware"
)

func TestRecoveryReturnsStandard500AndLogsOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// gin's recovery writes to DefaultErrorWriter, read when the middleware is created.
	var ginLog bytes.Buffer
	prevWriter := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &ginLog
	t.Cleanup(func() { gin.DefaultErrorWriter = prevWriter })

	var slogLog bytes.Buffer
	prevLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&slogLog, nil)))
	t.Cleanup(func() { slog.SetDefault(prevLogger) })

	r := gin.New()
	r.Use(middleware.Recovery())
	r.GET("/panic", func(*gin.Context) { panic("secret internal detail") })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/panic", http.NoBody))

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	var body apierror.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, apierror.CodeInternal, body.Error.Code)
	require.NotContains(t, rec.Body.String(), "secret", "panic value must not leak")

	require.Contains(t, ginLog.String(), "secret internal detail", "panic is logged by gin")
	require.Contains(t, ginLog.String(), "recovery_test.go", "gin's log includes the stack trace")
	require.Empty(t, slogLog.String(), "panic must not be logged a second time")
}

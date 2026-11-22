package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/dadamu/ggl-task-api/internal/transport/http/apierror"
	"github.com/dadamu/ggl-task-api/internal/transport/http/middleware"
)

func TestRequireJSON(t *testing.T) {
	tests := []struct {
		name        string
		contentType string // empty means the header is not set
		wantPass    bool
	}{
		{"json", "application/json", true},
		{"json with charset", "application/json; charset=utf-8", true},
		{"case-insensitive", "Application/JSON", true},
		{"missing", "", false},
		{"text/plain", "text/plain", false},
		{"form", "application/x-www-form-urlencoded", false},
		{"json suffix is not json", "application/problem+json", false},
		{"malformed", "application/json; =", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			reached := false
			r := gin.New()
			r.POST("/", middleware.RequireJSON(), func(c *gin.Context) {
				reached = true
				c.Status(http.StatusNoContent)
			})
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader(`{}`))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			rec := httptest.NewRecorder()

			r.ServeHTTP(rec, req)

			require.Equal(t, tt.wantPass, reached)
			if tt.wantPass {
				require.Equal(t, http.StatusNoContent, rec.Code)
				return
			}
			require.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
			var resp apierror.Response
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			require.Equal(t, apierror.CodeUnsupportedMediaType, resp.Error.Code)
		})
	}
}

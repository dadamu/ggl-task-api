package apierror_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/dadamu/ggl-task-api/internal/domain/task"
	"github.com/dadamu/ggl-task-api/internal/transport/http/apierror"
)

func init() {
	gin.SetMode(gin.TestMode)
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil))) // silence expected error logs
}

// run calls fn with a gin context and returns the recorded response.
func run(t *testing.T, fn func(c *gin.Context)) (int, apierror.Body) {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)

	fn(c)

	var resp apierror.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp), "body: %s", rec.Body.String())
	require.True(t, c.IsAborted(), "error responses must abort the chain")
	return rec.Code, resp.Error
}

func TestFromError(t *testing.T) {
	current := task.Task{ID: 7, Name: "a", Version: 3}
	validation := &task.ValidationError{Fields: []task.FieldError{{Field: "name", Reason: "must not be blank"}}}
	conflict := &task.VersionConflictError{Current: current}

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"validation", validation, http.StatusBadRequest, apierror.CodeValidationFailed},
		{"not found", task.ErrNotFound, http.StatusNotFound, apierror.CodeTaskNotFound},
		{"version conflict", conflict, http.StatusConflict, apierror.CodeVersionConflict},
		{"unknown", errors.New("secret"), http.StatusInternalServerError, apierror.CodeInternal},
		// Application code may wrap errors with %w (ADR-0004); mapping must still work.
		{"wrapped validation", fmt.Errorf("create: %w", validation), http.StatusBadRequest, apierror.CodeValidationFailed},
		{"wrapped not found", fmt.Errorf("update: %w", task.ErrNotFound), http.StatusNotFound, apierror.CodeTaskNotFound},
		{"wrapped conflict", fmt.Errorf("update: %w", conflict), http.StatusConflict, apierror.CodeVersionConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := run(t, func(c *gin.Context) { apierror.FromError(c, tt.err) })

			require.Equal(t, tt.wantStatus, status)
			require.Equal(t, tt.wantCode, body.Code)
			require.NotEmpty(t, body.Message)
			require.NotContains(t, body.Message, "secret", "internal details must not leak")
		})
	}
}

func TestFromErrorDetails(t *testing.T) {
	_, body := run(t, func(c *gin.Context) {
		apierror.FromError(c, &task.ValidationError{Fields: []task.FieldError{
			{Field: "name", Reason: "must not be blank"},
			{Field: "status", Reason: "must be 0 or 1"},
		}})
	})

	require.Equal(t, []apierror.FieldDetail{
		{Field: "name", Reason: "must not be blank"},
		{Field: "status", Reason: "must be 0 or 1"},
	}, body.Details)
	require.Nil(t, body.Current)
}

func TestFromErrorConflictCarriesCurrent(t *testing.T) {
	_, body := run(t, func(c *gin.Context) {
		apierror.FromError(c, &task.VersionConflictError{Current: task.Task{ID: 7, Name: "a", Version: 3}})
	})

	require.NotNil(t, body.Current)
	require.Equal(t, int64(7), body.Current.ID)
	require.Equal(t, int64(3), body.Current.Version)
	require.Empty(t, body.Details)
}

func TestFromBindErrorBodyTooLarge(t *testing.T) {
	err := fmt.Errorf("decode: %w", &http.MaxBytesError{Limit: 1 << 20})

	status, body := run(t, func(c *gin.Context) { apierror.FromBindError(c, err) })

	require.Equal(t, http.StatusRequestEntityTooLarge, status)
	require.Equal(t, apierror.CodePayloadTooLarge, body.Code)
}

func TestFromBindError(t *testing.T) {
	type target struct {
		I int     `json:"i"`
		S string  `json:"s"`
		B bool    `json:"b"`
		F float64 `json:"f"`
		P *int    `json:"p"`
	}
	decodeErr := func(body string) error {
		var v target
		return json.Unmarshal([]byte(body), &v)
	}

	tests := []struct {
		name        string
		err         error
		wantCode    string
		wantDetails []apierror.FieldDetail
	}{
		{"empty body", io.EOF, apierror.CodeInvalidJSON, nil},
		{"syntax error", decodeErr(`{"i":`), apierror.CodeInvalidJSON, nil},
		{"top-level type mismatch", decodeErr(`[]`), apierror.CodeInvalidJSON, nil},
		{"unknown error", errors.New("boom"), apierror.CodeInvalidJSON, nil},
		{
			"integer field", decodeErr(`{"i":"x"}`), apierror.CodeValidationFailed,
			[]apierror.FieldDetail{{Field: "i", Reason: "must be an integer"}},
		},
		{
			"pointer to integer field", decodeErr(`{"p":"x"}`), apierror.CodeValidationFailed,
			[]apierror.FieldDetail{{Field: "p", Reason: "must be an integer"}},
		},
		{
			"string field", decodeErr(`{"s":1}`), apierror.CodeValidationFailed,
			[]apierror.FieldDetail{{Field: "s", Reason: "must be a string"}},
		},
		{
			"other type", decodeErr(`{"b":"x"}`), apierror.CodeValidationFailed,
			[]apierror.FieldDetail{{Field: "b", Reason: "must be of type bool"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Error(t, tt.err)

			status, body := run(t, func(c *gin.Context) { apierror.FromBindError(c, tt.err) })

			require.Equal(t, http.StatusBadRequest, status)
			require.Equal(t, tt.wantCode, body.Code)
			require.Equal(t, tt.wantDetails, body.Details)
		})
	}
}

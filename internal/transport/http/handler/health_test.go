package handler_test

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/dadamu/ggl-task-api/internal/transport/http/handler"
)

func TestHealth(t *testing.T) {
	r := gin.New()
	r.GET("/healthz", handler.Health)

	rec := do(t, r, http.MethodGet, "/healthz", "")

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
}

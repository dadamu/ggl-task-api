package middleware

import (
	"mime"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/dadamu/ggl-task-api/internal/transport/http/apierror"
)

// RequireJSON rejects requests whose Content-Type is not application/json
// with 415 (ADR-0004). Parameters such as "; charset=utf-8" are allowed and
// the media type is matched case-insensitively. Attach it only to routes with
// a request body.
func RequireJSON() gin.HandlerFunc {
	return func(c *gin.Context) {
		mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
		if err != nil || mediaType != "application/json" {
			apierror.Abort(c, http.StatusUnsupportedMediaType, apierror.Body{
				Code:    apierror.CodeUnsupportedMediaType,
				Message: "Content-Type must be application/json",
			})
			return
		}
		c.Next()
	}
}

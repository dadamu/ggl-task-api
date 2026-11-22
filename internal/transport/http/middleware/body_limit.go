package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/dadamu/ggl-task-api/internal/transport/http/apierror"
)

// BodyLimit caps request bodies at maxBytes (ADR-0004).
//
// A request that declares a larger Content-Length is rejected with 413 before
// its body is read. Otherwise the body is wrapped with http.MaxBytesReader, so
// reading past the limit (e.g. chunked uploads) fails with *http.MaxBytesError,
// which apierror.FromBindError maps to 413.
func BodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > maxBytes {
			apierror.Abort(c, http.StatusRequestEntityTooLarge, apierror.Body{
				Code:    apierror.CodePayloadTooLarge,
				Message: "request body is too large",
			})
			return
		}
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}
		c.Next()
	}
}

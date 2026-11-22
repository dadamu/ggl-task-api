// Package middleware contains gin middlewares.
package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/dadamu/ggl-task-api/internal/transport/http/apierror"
)

// Recovery recovers from panics and responds with a 500 in the standard error
// format. gin's recovery logs the panic with its stack trace, so the response
// is written without logging again.
func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, _ any) {
		apierror.AbortInternal(c)
	})
}

// Package router builds the gin engine and registers routes.
package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	app "github.com/dadamu/ggl-task-api/internal/application/task"
	"github.com/dadamu/ggl-task-api/internal/transport/http/apierror"
	"github.com/dadamu/ggl-task-api/internal/transport/http/handler"
	"github.com/dadamu/ggl-task-api/internal/transport/http/middleware"
)

// MaxBodyBytes is the maximum request body size (ADR-0004).
const MaxBodyBytes = 1 << 20 // 1 MB

// Options configures optional routes.
type Options struct {
	// Swagger registers /swagger/* (ADR-0008). Disable it in production.
	Swagger bool
}

// New returns a gin engine with all routes registered.
func New(svc *app.Service, opts Options) *gin.Engine {
	r := gin.New()
	r.HandleMethodNotAllowed = true // 405 instead of 404 for a known path
	r.RedirectTrailingSlash = false // /tasks/ is a 404, not a redirect
	r.Use(gin.Logger(), middleware.Recovery(), middleware.BodyLimit(MaxBodyBytes))

	r.NoRoute(func(c *gin.Context) {
		apierror.Abort(c, http.StatusNotFound, apierror.Body{Code: apierror.CodeRouteNotFound, Message: "route not found"})
	})
	r.NoMethod(func(c *gin.Context) {
		apierror.Abort(c, http.StatusMethodNotAllowed, apierror.Body{Code: apierror.CodeMethodNotAllowed, Message: "method not allowed"})
	})

	r.GET("/healthz", handler.Health)
	if opts.Swagger {
		r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}

	tasks := handler.NewTask(svc)
	r.GET("/tasks", tasks.List)
	r.POST("/tasks", middleware.RequireJSON(), tasks.Create)
	r.PUT("/tasks/:id", middleware.RequireJSON(), tasks.Update)
	r.DELETE("/tasks/:id", tasks.Delete)

	return r
}

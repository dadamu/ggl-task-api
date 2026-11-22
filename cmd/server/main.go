// Package main is the entrypoint of the API server.
//
//	@title			GGL Task API
//	@version		1.0
//	@description	Task API built with DDD layout.
//	@BasePath		/
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	app "github.com/dadamu/ggl-task-api/internal/application/task"
	"github.com/dadamu/ggl-task-api/internal/infrastructures/config"
	"github.com/dadamu/ggl-task-api/internal/infrastructures/persistence/memory"
	"github.com/dadamu/ggl-task-api/internal/transport/http/router"
	_ "github.com/dadamu/ggl-task-api/swagger" // swagger docs
)

func main() {
	cfg := config.Load()
	gin.SetMode(cfg.GinMode)

	// Composition root: the only place that calls time.Now (ADR-0006).
	svc := app.NewService(memory.NewTaskRepository(), time.Now)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router.New(svc),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

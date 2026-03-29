package http

import (
	"fmt"
	"log/slog"
	nethttp "net/http"

	"smctf/internal/config"
	"smctf/internal/http/handlers"
	"smctf/internal/http/middleware"
	"smctf/internal/logging"
	"smctf/internal/stack"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func NewRouter(cfg config.Config, logger *logging.Logger, service *stack.Service) (*gin.Engine, error) {
	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	var log *slog.Logger
	if logger != nil {
		log = logger.Logger
	}

	if service == nil {
		return nil, fmt.Errorf("stack service is required")
	}

	h := handlers.New(service)

	r := gin.New()
	r.Use(middleware.RecoveryLogger(logger))
	r.Use(middleware.RequestLogger(cfg.Logging, logger))

	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	if !cfg.APIKey.Enabled && log != nil {
		log.Warn("api key auth disabled")
	}

	api := r.Group("/")
	api.Use(middleware.APIKeyAuth(cfg.APIKey))

	api.GET("/healthz", func(c *gin.Context) {
		c.JSON(nethttp.StatusOK, gin.H{"status": "ok"})
	})

	api.POST("/stacks", h.CreateStack)
	api.GET("/stacks", h.ListStacks)
	api.GET("/stacks/:stack_id", h.GetStack)
	api.GET("/stacks/:stack_id/status", h.GetStackStatusSummary)
	api.DELETE("/stacks/:stack_id", h.DeleteStack)
	api.POST("/stacks/batch-delete", h.CreateBatchDeleteJob)
	api.GET("/stacks/batch-delete/:job_id", h.GetBatchDeleteJob)
	api.GET("/stats", h.GetStats)

	attachFrontendRoutes(r)

	return r, nil
}

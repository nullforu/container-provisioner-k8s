package http

import (
	"context"
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

func NewRouter(ctx context.Context, cfg config.Config, logger *logging.Logger) (*gin.Engine, error) {
	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	var log *slog.Logger
	if logger != nil {
		log = logger.Logger
	}

	repo, err := stack.NewRepositoryFromConfig(ctx, cfg.Stack)
	if err != nil {
		return nil, fmt.Errorf("init repository: %w", err)
	}

	k8s, err := stack.NewKubernetesClientFromConfig(cfg.Stack)
	if err != nil {
		return nil, fmt.Errorf("init kubernetes client: %w", err)
	}

	if cfg.Stack.RequireIngressNP {
		ok, err := k8s.HasIngressNetworkPolicy(ctx)
		if err != nil {
			return nil, fmt.Errorf("check ingress networkpolicy: %w", err)
		}

		if !ok {
			return nil, fmt.Errorf("missing ingress networkpolicy")
		}
	}

	if count, err := k8s.CountSchedulableNodes(ctx); err != nil {
		if log != nil {
			log.Warn("count schedulable nodes failed", slog.Any("error", err))
		}
	} else if log != nil {
		log.Info("schedulable nodes detected", slog.Int("count", count), slog.String("role", cfg.Stack.StackNodeRole))
	}

	service := stack.NewService(cfg.Stack, repo, k8s)
	scheduler := stack.NewScheduler(cfg.Stack.SchedulerInterval, service)
	go scheduler.Run(ctx)

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

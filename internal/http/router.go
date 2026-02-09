package http

import (
	"context"
	"fmt"
	"io"
	nethttp "net/http"
	"os"

	"smctf/internal/config"
	"smctf/internal/http/handlers"
	"smctf/internal/http/middleware"
	"smctf/internal/logging"
	"smctf/internal/stack"

	"github.com/gin-gonic/gin"
)

func NewRouter(ctx context.Context, cfg config.Config, logger *logging.Logger) (*gin.Engine, error) {
	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	if logger != nil {
		gin.DefaultWriter = io.MultiWriter(os.Stdout, logger)
		gin.DefaultErrorWriter = io.MultiWriter(os.Stderr, logger)
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
		fmt.Printf("level=WARN msg=\"count schedulable nodes failed\" err=%q\n", err.Error())
	} else {
		fmt.Printf("level=INFO msg=\"schedulable nodes detected\" count=%d role=%s\n", count, cfg.Stack.StackNodeRole)
	}

	service := stack.NewService(cfg.Stack, repo, k8s)
	scheduler := stack.NewScheduler(cfg.Stack.SchedulerInterval, service)
	go scheduler.Run(ctx)

	h := handlers.New(service)

	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.Use(middleware.RequestLogger(cfg.Logging, logger))

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(nethttp.StatusOK, gin.H{"status": "ok"})
	})

	r.POST("/stacks", h.CreateStack)
	r.GET("/stacks", h.ListStacks)
	r.GET("/stacks/:stack_id", h.GetStack)
	r.GET("/stacks/:stack_id/status", h.GetStackStatus)
	r.DELETE("/stacks/:stack_id", h.DeleteStack)
	r.GET("/stats", h.GetStats)

	attachFrontendRoutes(r)

	return r, nil
}

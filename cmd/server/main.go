package main

import (
	"context"
	"log/slog"
	nethttp "net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"smctf/internal/config"
	httpserver "smctf/internal/http"
	"smctf/internal/logging"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		boot := slog.New(slog.NewJSONHandler(os.Stderr, nil))
		boot.Error("config error", slog.Any("error", err))
		os.Exit(1)
	}

	logger, err := logging.New(cfg.Logging, logging.Options{
		Service:   "container-provisioner",
		Env:       cfg.AppEnv,
		AddSource: false,
	})
	if err != nil {
		boot := slog.New(slog.NewJSONHandler(os.Stderr, nil))
		boot.Error("logging init error", slog.Any("error", err))
		os.Exit(1)
	}

	slog.SetDefault(logger.Logger)

	defer func() {
		if err := logger.Close(); err != nil {
			logger.Error("log close error", slog.Any("error", err))
		}
	}()

	logger.Info("config loaded", slog.Any("config", config.FormatForLog(cfg)))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	router, err := httpserver.NewRouter(ctx, cfg, logger)
	if err != nil {
		logger.Error("router init error", slog.Any("error", err))
		os.Exit(1)
	}

	srv := &nethttp.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("server listening", slog.String("addr", cfg.HTTPAddr))
		if err := srv.ListenAndServe(); err != nil && err != nethttp.ErrServerClosed {
			logger.Error("server error", slog.Any("error", err))
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", slog.Any("error", err))
	}
}

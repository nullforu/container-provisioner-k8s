package main

import (
	"context"
	"log/slog"
	"net"
	nethttp "net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"smctf/internal/bootstrap"
	"smctf/internal/config"
	stackv1 "smctf/internal/gen/stack/v1"
	"smctf/internal/grpcserver"
	httpserver "smctf/internal/http"
	"smctf/internal/logging"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
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

	service, err := bootstrap.BootstrapStackService(ctx, cfg, logger)
	if err != nil {
		logger.Error("stack service init error", slog.Any("error", err))
		os.Exit(1)
	}

	router, err := httpserver.NewRouter(cfg, logger, service)
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

	var grpcServer *grpc.Server
	var grpcListener net.Listener

	if cfg.GRPCEnabled {
		grpcListener, err = net.Listen("tcp", cfg.GRPCAddr)
		if err != nil {
			logger.Error("grpc listen error", slog.Any("error", err))
			os.Exit(1)
		}

		grpcServer = grpc.NewServer(
			grpc.UnaryInterceptor(grpcserver.APIKeyUnaryInterceptor(cfg.APIKey)),
		)

		stackv1.RegisterStackServiceServer(grpcServer, grpcserver.New(service, logger.Logger))
		if cfg.GRPCReflectionEnabled {
			reflection.Register(grpcServer)
		}

		go func() {
			logger.Info("grpc server listening", slog.String("addr", cfg.GRPCAddr))
			if err := grpcServer.Serve(grpcListener); err != nil && err != grpc.ErrServerStopped {
				logger.Error("grpc server error", slog.Any("error", err))
			}
		}()
	}

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", slog.Any("error", err))
	}

	if grpcServer != nil {
		done := make(chan struct{})
		go func() {
			grpcServer.GracefulStop()
			close(done)
		}()

		select {
		case <-done:
		case <-shutdownCtx.Done():
			grpcServer.Stop()
		}
	}
}

package bootstrap

import (
	"context"
	"fmt"
	"log/slog"

	"smctf/internal/config"
	"smctf/internal/logging"
	"smctf/internal/stack"
)

func BootstrapStackService(ctx context.Context, cfg config.Config, logger *logging.Logger) (*stack.Service, error) {
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
	if cfg.Stack.LeaderElection.Enabled {
		if cfg.Stack.UseMockKubernetes {
			if log != nil {
				log.Warn("leader election disabled because K8S_USE_MOCK=true",
					slog.String("namespace", cfg.Stack.LeaderElection.Namespace),
					slog.String("name", cfg.Stack.LeaderElection.LeaseName),
				)
			}

			go scheduler.Run(ctx)
		} else {
			if log != nil {
				log.Info("leader election enabled",
					slog.String("namespace", cfg.Stack.LeaderElection.Namespace),
					slog.String("name", cfg.Stack.LeaderElection.LeaseName),
				)
			}

			if err := stack.StartLeaderElection(ctx, cfg.Stack, log, func(leaderCtx context.Context) {
				scheduler.Run(leaderCtx)
			}); err != nil {
				return nil, fmt.Errorf("start leader election: %w", err)
			}
		}
	} else {
		go scheduler.Run(ctx)
	}

	return service, nil
}

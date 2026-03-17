package stack

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"smctf/internal/config"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

func StartLeaderElection(ctx context.Context, cfg config.StackConfig, log *slog.Logger, onStarted func(context.Context)) error {
	if onStarted == nil {
		return fmt.Errorf("leader election start requires onStarted callback")
	}

	leaderLog(log, slog.LevelInfo, "leader election configured",
		slog.Bool("enabled", cfg.LeaderElection.Enabled),
		slog.String("namespace", cfg.LeaderElection.Namespace),
		slog.String("name", cfg.LeaderElection.LeaseName),
		slog.Duration("lease_duration", cfg.LeaderElection.LeaseDuration),
		slog.Duration("renew_deadline", cfg.LeaderElection.RenewDeadline),
		slog.Duration("retry_period", cfg.LeaderElection.RetryPeriod),
	)

	restCfg, err := buildKubeConfig(cfg)
	if err != nil {
		return fmt.Errorf("build kube config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("new kubernetes client: %w", err)
	}

	identity := leaderIdentity()
	leaderLog(log, slog.LevelInfo, "leader election identity resolved", slog.String("identity", identity))

	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      cfg.LeaderElection.LeaseName,
			Namespace: cfg.LeaderElection.Namespace,
		},
		Client: clientset.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: identity,
		},
	}

	leaderElector, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock:            lock,
		ReleaseOnCancel: true,
		LeaseDuration:   cfg.LeaderElection.LeaseDuration,
		RenewDeadline:   cfg.LeaderElection.RenewDeadline,
		RetryPeriod:     cfg.LeaderElection.RetryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(leaderCtx context.Context) {
				leaderLog(log, slog.LevelInfo, "leader election acquired")
				onStarted(leaderCtx)
			},
			OnStoppedLeading: func() {
				leaderLog(log, slog.LevelWarn, "leader election lost")
			},
			OnNewLeader: func(identity string) {
				if identity == "" {
					return
				}

				leaderLog(log, slog.LevelInfo, "leader elected", slog.String("identity", identity))
			},
		},
	})
	if err != nil {
		return fmt.Errorf("create leader elector: %w", err)
	}

	go leaderElector.Run(ctx)

	return nil
}

func leaderIdentity() string {
	if v := os.Getenv("LEADER_ELECTION_IDENTITY"); v != "" {
		return v
	}

	if v := os.Getenv("POD_NAME"); v != "" {
		return v
	}

	return fmt.Sprintf("leader-%d-%d", os.Getpid(), time.Now().UnixNano())
}

func leaderLog(log *slog.Logger, level slog.Level, msg string, attrs ...slog.Attr) {
	if log == nil {
		return
	}

	log.LogAttrs(context.Background(), level, msg, attrs...)
}

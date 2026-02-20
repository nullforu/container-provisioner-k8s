package stack

import (
	"context"
	"log/slog"
	"time"
)

type Scheduler struct {
	interval time.Duration
	service  *Service
}

func NewScheduler(interval time.Duration, service *Service) *Scheduler {
	return &Scheduler{interval: interval, service: service}
}

func (s *Scheduler) Run(ctx context.Context) {
	if s == nil || s.service == nil {
		slog.Error("scheduler run skipped due to nil dependency")
		return
	}

	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("scheduler panic recovered", slog.Any("error", rec))
		}
	}()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	s.service.CleanupExpiredAndOrphaned(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.service.CleanupExpiredAndOrphaned(ctx)
		}
	}
}

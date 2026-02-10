package stack

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"smctf/internal/config"
)

type Service struct {
	cfg       config.StackConfig
	repo      RepositoryClientAPI
	k8s       KubernetesClientAPI
	validator *Validator
	now       func() time.Time
}

func NewService(cfg config.StackConfig, repo RepositoryClientAPI, k8s KubernetesClientAPI) *Service {
	return &Service{
		cfg:       cfg,
		repo:      repo,
		k8s:       k8s,
		validator: NewValidator(cfg),
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (s *Service) Create(ctx context.Context, in CreateInput) (Stack, error) {
	valid, err := s.validator.ValidatePodSpec(in.PodSpecYML, in.TargetPort)
	if err != nil {
		return Stack{}, err
	}

	nodePort, err := s.repo.ReserveNodePort(ctx, s.cfg.NodePortMin, s.cfg.NodePortMax)
	if err != nil {
		return Stack{}, err
	}

	releasePort := true
	defer func() {
		if releasePort {
			if err := s.repo.ReleaseNodePort(context.Background(), nodePort); err != nil {
				log.Printf("level=ERROR msg=\"release reserved nodeport failed\" node_port=%d err=%q", nodePort, err.Error())
			}
		}
	}()

	stackID := newStackID()
	now := s.now()
	st := Stack{
		StackID:        stackID,
		Namespace:      s.cfg.Namespace,
		PodSpecYAML:    valid.SanitizedYAML,
		TargetPort:     in.TargetPort,
		NodePort:       nodePort,
		Status:         StatusCreating,
		CreatedAt:      now,
		UpdatedAt:      now,
		TTLExpiresAt:   now.Add(s.cfg.StackTTL),
		RequestedMilli: valid.RequestedMilli,
		RequestedBytes: valid.RequestedBytes,
	}

	result, err := s.k8s.CreatePodAndService(ctx, ProvisionRequest{
		Namespace:  s.cfg.Namespace,
		StackID:    stackID,
		PodSpecYML: valid.SanitizedYAML,
		TargetPort: in.TargetPort,
		NodePort:   nodePort,
	})
	if err != nil {
		return Stack{}, mapProvisionError(err)
	}

	st.PodID = result.PodID
	st.ServiceName = result.ServiceName
	st.NodeID = result.NodeID
	st.Status = result.Status

	nodePublicIP, ipErr := s.k8s.GetNodePublicIP(ctx, st.NodeID)
	if ipErr != nil {
		log.Printf("level=WARN msg=\"resolve node public ip failed\" stack_id=%s node_id=%s err=%q", st.StackID, st.NodeID, ipErr.Error())
	}
	st.NodePublicIP = nodePublicIP

	if err := s.repo.Create(ctx, st); err != nil {
		if k8sErr := s.k8s.DeletePodAndService(context.Background(), st.Namespace, st.PodID, st.ServiceName); k8sErr != nil {
			log.Printf("level=ERROR msg=\"rollback delete pod/service failed\" stack_id=%s pod_id=%s service_name=%s err=%q", st.StackID, st.PodID, st.ServiceName, k8sErr.Error())
		}

		return Stack{}, err
	}

	releasePort = false

	return st, nil
}

func (s *Service) GetDetails(ctx context.Context, stackID string) (Stack, error) {
	if err := s.RefreshStatus(ctx, stackID); err != nil {
		return Stack{}, err
	}

	st, ok, err := s.repo.Get(ctx, stackID)
	if err != nil {
		return Stack{}, err
	}

	if !ok {
		return Stack{}, ErrNotFound
	}

	s.attachNodePublicIP(ctx, &st)
	return st, nil
}

func (s *Service) GetStatus(ctx context.Context, stackID string) (Status, error) {
	summary, err := s.GetStatusSummary(ctx, stackID)
	if err != nil {
		return "", err
	}

	return summary.Status, nil
}

func (s *Service) GetStatusSummary(ctx context.Context, stackID string) (StackStatusSummary, error) {
	if err := s.RefreshStatus(ctx, stackID); err != nil {
		return StackStatusSummary{}, err
	}

	st, ok, err := s.repo.Get(ctx, stackID)
	if err != nil {
		return StackStatusSummary{}, err
	}

	if !ok {
		return StackStatusSummary{}, ErrNotFound
	}

	return StackStatusSummary{
		StackID:      st.StackID,
		Status:       st.Status,
		TTL:          st.TTLExpiresAt,
		NodePort:     st.NodePort,
		TargetPort:   st.TargetPort,
		NodePublicIP: s.nodePublicIP(ctx, st.NodeID),
	}, nil
}

func (s *Service) RefreshStatus(ctx context.Context, stackID string) error {
	st, ok, err := s.repo.Get(ctx, stackID)
	if err != nil {
		return err
	}

	if !ok {
		return ErrNotFound
	}

	nodeExists, err := s.k8s.NodeExists(ctx, st.NodeID)
	if err != nil {
		return err
	}

	if !nodeExists {
		if err := s.k8s.DeletePodAndService(ctx, st.Namespace, st.PodID, st.ServiceName); err != nil {
			log.Printf("level=ERROR msg=\"delete pod/service on missing node failed\" stack_id=%s pod_id=%s service_name=%s err=%q", st.StackID, st.PodID, st.ServiceName, err.Error())
		}

		if _, _, err := s.repo.Delete(ctx, st.StackID); err != nil {
			log.Printf("level=ERROR msg=\"delete stack from repository on missing node failed\" stack_id=%s err=%q", st.StackID, err.Error())
		}

		return ErrNotFound
	}

	status, nodeID, err := s.k8s.GetPodStatus(ctx, st.Namespace, st.PodID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			if _, _, deleteErr := s.repo.Delete(ctx, st.StackID); deleteErr != nil {
				log.Printf("level=ERROR msg=\"delete stack after missing pod failed\" stack_id=%s err=%q", st.StackID, deleteErr.Error())
			}

			return ErrNotFound
		}

		return err
	}

	if status == StatusNodeDeleted {
		if err := s.k8s.DeletePodAndService(ctx, st.Namespace, st.PodID, st.ServiceName); err != nil {
			log.Printf("level=ERROR msg=\"delete pod/service on node_deleted failed\" stack_id=%s pod_id=%s service_name=%s err=%q", st.StackID, st.PodID, st.ServiceName, err.Error())
		}

		if _, _, err := s.repo.Delete(ctx, st.StackID); err != nil {
			log.Printf("level=ERROR msg=\"delete stack from repository on node_deleted failed\" stack_id=%s err=%q", st.StackID, err.Error())
		}

		return ErrNotFound
	}

	if err := s.repo.UpdateStatus(ctx, st.StackID, status, nodeID); err != nil {
		log.Printf("level=ERROR msg=\"update stack status failed\" stack_id=%s status=%s node_id=%s err=%q", st.StackID, status, nodeID, err.Error())
	}

	return nil
}

func (s *Service) Delete(ctx context.Context, stackID string) error {
	st, ok, err := s.repo.Get(ctx, stackID)
	if err != nil {
		return err
	}

	if !ok {
		return ErrNotFound
	}

	if err := s.k8s.DeletePodAndService(ctx, st.Namespace, st.PodID, st.ServiceName); err != nil {
		log.Printf("level=ERROR msg=\"delete pod/service failed\" stack_id=%s pod_id=%s service_name=%s err=%q", st.StackID, st.PodID, st.ServiceName, err.Error())
	}

	_, _, err = s.repo.Delete(ctx, stackID)

	return err
}

func (s *Service) ListAll(ctx context.Context) ([]Stack, error) {
	items, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}

	refreshed := make([]Stack, 0, len(items))
	for _, item := range items {
		if err := s.RefreshStatus(ctx, item.StackID); err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}

		st, ok, err := s.repo.Get(ctx, item.StackID)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}

		s.attachNodePublicIP(ctx, &st)
		refreshed = append(refreshed, st)
	}

	return refreshed, nil
}

func (s *Service) Stats(ctx context.Context) (Stats, error) {
	items, err := s.ListAll(ctx)
	if err != nil {
		return Stats{}, err
	}

	usedPorts, err := s.repo.UsedNodePortCount(ctx)
	if err != nil {
		return Stats{}, err
	}

	stats := Stats{
		NodeDistribution: make(map[string]int),
		UsedNodePorts:    usedPorts,
	}

	for _, st := range items {
		stats.TotalStacks++
		if st.Status == StatusRunning || st.Status == StatusCreating {
			stats.ActiveStacks++
		}
		stats.NodeDistribution[st.NodeID]++
		stats.ReservedCPUMilli += st.RequestedMilli
		stats.ReservedMemoryBytes += st.RequestedBytes
	}

	return stats, nil
}

func (s *Service) attachNodePublicIP(ctx context.Context, st *Stack) {
	if st == nil {
		return
	}

	st.NodePublicIP = s.nodePublicIP(ctx, st.NodeID)
}

func (s *Service) nodePublicIP(ctx context.Context, nodeID string) *string {
	ip, err := s.k8s.GetNodePublicIP(ctx, nodeID)
	if err != nil {
		log.Printf("level=WARN msg=\"resolve node public ip failed\" node_id=%s err=%q", nodeID, err.Error())
		return nil
	}
	return ip
}

func (s *Service) CleanupExpiredAndOrphaned(ctx context.Context) {
	now := s.now()
	items, err := s.ListAll(ctx)
	if err != nil {
		log.Printf("level=ERROR msg=\"list stacks for cleanup failed\" err=%q", err.Error())
		log.Printf("level=INFO msg=\"cleanup loop completed\" scanned=0 targets=0 cleaned=0 failures=1 resource_scan_errors=0 orphan_scan_errors=0 note=%q", "list stacks failed")

		return
	}

	scanned := len(items)
	expiredTargets := 0
	missingResourceTargets := 0
	orphanPodTargets := 0
	cleaned := 0
	failures := 0
	resourceScanErrors := 0
	orphanScanErrors := 0

	for _, st := range items {
		if st.TTLExpiresAt.Before(now) || st.TTLExpiresAt.Equal(now) {
			expiredTargets++
			failed := false
			if err := s.k8s.DeletePodAndService(ctx, st.Namespace, st.PodID, st.ServiceName); err != nil {
				log.Printf("level=ERROR msg=\"cleanup delete pod/service failed\" stack_id=%s pod_id=%s service_name=%s err=%q", st.StackID, st.PodID, st.ServiceName, err.Error())
				failed = true
			}

			if _, _, err := s.repo.Delete(ctx, st.StackID); err != nil {
				log.Printf("level=ERROR msg=\"cleanup delete stack from repository failed\" stack_id=%s err=%q", st.StackID, err.Error())
				failed = true
			}

			if failed {
				failures++
			} else {
				cleaned++
			}
		}
	}

	remainingStacks, err := s.ListAll(ctx)
	if err != nil {
		orphanScanErrors++
		failures++
		log.Printf("level=ERROR msg=\"list stacks for orphan pod cleanup failed\" err=%q", err.Error())
	} else {
		podIDs, podErr := s.k8s.ListPods(ctx, s.cfg.Namespace)
		if podErr != nil {
			resourceScanErrors++
			failures++
			log.Printf("level=ERROR msg=\"list kubernetes pods for stack resource integrity failed\" namespace=%s err=%q", s.cfg.Namespace, podErr.Error())
		}

		serviceNames, svcErr := s.k8s.ListServices(ctx, s.cfg.Namespace)
		if svcErr != nil {
			resourceScanErrors++
			failures++
			log.Printf("level=ERROR msg=\"list kubernetes services for stack resource integrity failed\" namespace=%s err=%q", s.cfg.Namespace, svcErr.Error())
		}

		if podErr == nil && svcErr == nil {
			podSet := make(map[string]struct{}, len(podIDs))
			for _, podID := range podIDs {
				podSet[podID] = struct{}{}
			}

			serviceSet := make(map[string]struct{}, len(serviceNames))
			for _, serviceName := range serviceNames {
				serviceSet[serviceName] = struct{}{}
			}

			for _, st := range remainingStacks {
				_, podExists := podSet[st.PodID]
				_, serviceExists := serviceSet[st.ServiceName]
				if podExists && serviceExists {
					continue
				}

				missingResourceTargets++
				failed := false
				if err := s.k8s.DeletePodAndService(ctx, st.Namespace, st.PodID, st.ServiceName); err != nil {
					log.Printf("level=ERROR msg=\"cleanup delete stale stack resources failed\" stack_id=%s pod_id=%s service_name=%s err=%q", st.StackID, st.PodID, st.ServiceName, err.Error())
					failed = true
				}

				if _, _, err := s.repo.Delete(ctx, st.StackID); err != nil {
					log.Printf("level=ERROR msg=\"cleanup delete stack with missing pod/service failed\" stack_id=%s pod_exists=%t service_exists=%t err=%q", st.StackID, podExists, serviceExists, err.Error())
					failed = true
				}

				if failed {
					failures++
				} else {
					cleaned++
				}
			}
		}

		remainingStacks, err = s.ListAll(ctx)
		if err != nil {
			orphanScanErrors++
			failures++
			log.Printf("level=ERROR msg=\"list stacks after resource integrity cleanup failed\" err=%q", err.Error())
		} else {
			registeredPods := make(map[string]struct{}, len(remainingStacks))
			for _, st := range remainingStacks {
				if st.PodID == "" {
					continue
				}

				registeredPods[st.PodID] = struct{}{}
			}

			podIDs, err := s.k8s.ListPods(ctx, s.cfg.Namespace)
			if err != nil {
				orphanScanErrors++
				failures++
				log.Printf("level=ERROR msg=\"list kubernetes pods for orphan cleanup failed\" namespace=%s err=%q", s.cfg.Namespace, err.Error())
			} else {
				for _, podID := range podIDs {
					if _, ok := registeredPods[podID]; ok {
						continue
					}

					orphanPodTargets++
					if err := s.k8s.DeletePodAndService(ctx, s.cfg.Namespace, podID, ""); err != nil {
						failures++
						log.Printf("level=ERROR msg=\"cleanup delete orphan pod failed\" namespace=%s pod_id=%s err=%q", s.cfg.Namespace, podID, err.Error())
						continue
					}

					cleaned++
				}
			}
		}
	}

	targets := expiredTargets + missingResourceTargets + orphanPodTargets
	if targets == 0 {
		log.Printf("level=INFO msg=\"cleanup loop completed\" scanned=%d targets=0 cleaned=0 failures=%d resource_scan_errors=%d orphan_scan_errors=%d note=%q", scanned, failures, resourceScanErrors, orphanScanErrors, "no cleanup candidates")

		return
	}

	log.Printf(
		"level=INFO msg=\"cleanup loop completed\" scanned=%d targets=%d expired_targets=%d missing_resource_targets=%d orphan_pod_targets=%d cleaned=%d failures=%d resource_scan_errors=%d orphan_scan_errors=%d",
		scanned,
		targets,
		expiredTargets,
		missingResourceTargets,
		orphanPodTargets,
		cleaned,
		failures,
		resourceScanErrors,
		orphanScanErrors,
	)
}

func newStackID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("stack-%d", time.Now().UnixNano())
	}

	return "stack-" + hex.EncodeToString(buf)
}

func mapProvisionError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, ErrClusterSaturated) || errors.Is(err, ErrPodSpecInvalid) {
		return err
	}

	lowerMsg := strings.ToLower(err.Error())
	if isQuotaExceededMessage(lowerMsg) {
		return fmt.Errorf("%w: %v", ErrClusterSaturated, err)
	}

	if isLimitRangeExceededMessage(lowerMsg) {
		return fmt.Errorf("%w: %v", ErrPodSpecInvalid, err)
	}

	return fmt.Errorf("k8s provision failed: %w", err)
}

func isQuotaExceededMessage(msg string) bool {
	return strings.Contains(msg, "exceeded quota") ||
		strings.Contains(msg, "exceeds quota") ||
		strings.Contains(msg, "resourcequota")
}

func isLimitRangeExceededMessage(msg string) bool {
	return strings.Contains(msg, "limitrange") ||
		strings.Contains(msg, "limit range") ||
		(strings.Contains(msg, "per container") && strings.Contains(msg, "limit is")) ||
		strings.Contains(msg, "must be less than or equal to") ||
		strings.Contains(msg, "must be greater than or equal to")
}

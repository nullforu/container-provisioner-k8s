package stack

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"smctf/internal/config"
)

func TestServiceCreateAndDelete(t *testing.T) {
	repo := NewInMemoryRepository(1)
	k8s := NewMockKubernetesClient(1)
	svc := NewService(config.StackConfig{
		Namespace:         "stacks",
		StackTTL:          time.Hour,
		SchedulerInterval: time.Second,
		NodePortMin:       30000,
		NodePortMax:       30010,
	}, repo, k8s)

	st, err := svc.Create(context.Background(), CreateInput{
		TargetPorts: []PortSpec{{ContainerPort: 5000, Protocol: "TCP"}},
		PodSpecYML: `
apiVersion: v1
kind: Pod
metadata:
  name: p
spec:
  containers:
    - name: app
      image: nginx:latest
      ports:
        - containerPort: 5000
      resources:
        limits:
          cpu: "500m"
          memory: "256Mi"
`,
	})
	if err != nil {
		t.Fatalf("create error: %v", err)
	}

	if len(st.Ports) != 1 {
		t.Fatalf("expected 1 port mapping, got %d", len(st.Ports))
	}

	if st.Ports[0].NodePort < 30000 || st.Ports[0].NodePort > 30010 {
		t.Fatalf("unexpected node port: %d", st.Ports[0].NodePort)
	}

	status, err := svc.GetStatus(context.Background(), st.StackID)
	if err != nil {
		t.Fatalf("status error: %v", err)
	}

	if status != StatusRunning {
		t.Fatalf("expected running, got %s", status)
	}

	if err := svc.Delete(context.Background(), st.StackID); err != nil {
		t.Fatalf("delete error: %v", err)
	}

	if _, err := svc.GetDetails(context.Background(), st.StackID); err == nil {
		t.Fatalf("expected not found")
	}
}

func TestServiceCreateWithMultiplePorts(t *testing.T) {
	repo := NewInMemoryRepository(1)
	k8s := NewMockKubernetesClient(1)
	svc := NewService(config.StackConfig{
		Namespace:         "stacks",
		StackTTL:          time.Hour,
		SchedulerInterval: time.Second,
		NodePortMin:       30000,
		NodePortMax:       30010,
	}, repo, k8s)

	st, err := svc.Create(context.Background(), CreateInput{
		TargetPorts: []PortSpec{
			{ContainerPort: 5000, Protocol: "TCP"},
			{ContainerPort: 5001, Protocol: "UDP"},
		},
		PodSpecYML: `
apiVersion: v1
kind: Pod
metadata:
  name: p
spec:
  containers:
    - name: app
      image: nginx:latest
      ports:
        - containerPort: 5000
          protocol: TCP
        - containerPort: 5001
          protocol: UDP
      resources:
        limits:
          cpu: "500m"
          memory: "256Mi"
`,
	})
	if err != nil {
		t.Fatalf("create error: %v", err)
	}

	if len(st.Ports) != 2 {
		t.Fatalf("expected 2 port mappings, got %d", len(st.Ports))
	}

	if st.Ports[0].NodePort == st.Ports[1].NodePort {
		t.Fatalf("expected distinct node ports")
	}
}

type retryingKubernetesClient struct {
	attempts int
}

func (r *retryingKubernetesClient) CreatePodAndService(_ context.Context, _ ProvisionRequest) (ProvisionResult, error) {
	r.attempts++
	if r.attempts == 1 {
		return ProvisionResult{}, fmt.Errorf("provided port is already allocated")
	}

	return ProvisionResult{
		PodID:       "pod-ok",
		ServiceName: "svc-ok",
		NodeID:      "worker-a",
		Status:      StatusRunning,
	}, nil
}

func (r *retryingKubernetesClient) DeletePodAndService(_ context.Context, _, _, _ string) error {
	return nil
}

func (r *retryingKubernetesClient) GetPodStatus(_ context.Context, _, _ string) (Status, string, error) {
	return StatusRunning, "worker-a", nil
}

func (r *retryingKubernetesClient) ListPods(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func (r *retryingKubernetesClient) ListPodsWithCreation(_ context.Context, _ string) (map[string]PodInfo, error) {
	return map[string]PodInfo{}, nil
}

func (r *retryingKubernetesClient) ListServices(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func (r *retryingKubernetesClient) NodeExists(_ context.Context, _ string) (bool, error) {
	return true, nil
}

func (r *retryingKubernetesClient) HasIngressNetworkPolicy(_ context.Context) (bool, error) {
	return true, nil
}

func (r *retryingKubernetesClient) GetNodePublicIP(_ context.Context, _ string) (*string, error) {
	return nil, nil
}

func (r *retryingKubernetesClient) CountSchedulableNodes(_ context.Context) (int, error) {
	return 1, nil
}

func TestServiceCreateRetriesOnNodePortAllocated(t *testing.T) {
	repo := NewInMemoryRepository(1)
	k8s := &retryingKubernetesClient{}
	svc := NewService(config.StackConfig{
		Namespace:         "stacks",
		StackTTL:          time.Hour,
		SchedulerInterval: time.Second,
		NodePortMin:       30000,
		NodePortMax:       30010,
	}, repo, k8s)

	st, err := svc.Create(context.Background(), CreateInput{
		TargetPorts: []PortSpec{{ContainerPort: 5000, Protocol: "TCP"}},
		PodSpecYML: `
apiVersion: v1
kind: Pod
metadata:
  name: p
spec:
  containers:
    - name: app
      image: nginx:latest
      ports:
        - containerPort: 5000
      resources:
        limits:
          cpu: "500m"
          memory: "256Mi"
`,
	})
	if err != nil {
		t.Fatalf("create error: %v", err)
	}

	if k8s.attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", k8s.attempts)
	}

	used, err := repo.UsedNodePortCount(context.Background())
	if err != nil {
		t.Fatalf("used node ports error: %v", err)
	}

	if used != len(st.Ports) {
		t.Fatalf("expected %d used ports, got %d", len(st.Ports), used)
	}
}

func TestCleanupRemovesOnlyPodsMissingFromRepository(t *testing.T) {
	repo := NewInMemoryRepository(1)
	k8s := NewMockKubernetesClient(1)
	svc := NewService(config.StackConfig{
		Namespace:         "stacks",
		StackTTL:          time.Hour,
		SchedulerInterval: time.Second,
		NodePortMin:       30000,
		NodePortMax:       30010,
	}, repo, k8s)

	st, err := svc.Create(context.Background(), CreateInput{
		TargetPorts: []PortSpec{{ContainerPort: 5000, Protocol: "TCP"}},
		PodSpecYML: `
apiVersion: v1
kind: Pod
metadata:
  name: p
spec:
  containers:
    - name: app
      image: nginx:latest
      ports:
        - containerPort: 5000
      resources:
        limits:
          cpu: "100m"
          memory: "64Mi"
`,
	})
	if err != nil {
		t.Fatalf("create error: %v", err)
	}

	k8s.mu.Lock()
	k8s.pods["orphan-pod"] = podState{
		namespace: "stacks",
		podID:     "orphan-pod",
		service:   "svc-orphan-pod",
		nodeID:    "worker-a",
		status:    StatusRunning,
		createdAt: time.Now().UTC().Add(-3 * time.Minute),
		stackID:   "",
	}
	k8s.pods["other-ns-pod"] = podState{
		namespace: "other",
		podID:     "other-ns-pod",
		service:   "svc-other-ns-pod",
		nodeID:    "worker-a",
		status:    StatusRunning,
		createdAt: time.Now().UTC().Add(-3 * time.Minute),
		stackID:   "",
	}
	k8s.mu.Unlock()

	svc.CleanupExpiredAndOrphaned(context.Background())

	podsInStacks, err := k8s.ListPods(context.Background(), "stacks")
	if err != nil {
		t.Fatalf("list stacks namespace pods error: %v", err)
	}

	hasManagedPod := false
	hasOrphanPod := false
	for _, podID := range podsInStacks {
		if podID == st.PodID {
			hasManagedPod = true
		}
		if podID == "orphan-pod" {
			hasOrphanPod = true
		}
	}

	if !hasManagedPod {
		t.Fatalf("managed pod should remain after cleanup")
	}

	if hasOrphanPod {
		t.Fatalf("orphan pod should be deleted during cleanup")
	}

	podsInOther, err := k8s.ListPods(context.Background(), "other")
	if err != nil {
		t.Fatalf("list other namespace pods error: %v", err)
	}

	if len(podsInOther) != 1 || podsInOther[0] != "other-ns-pod" {
		t.Fatalf("cleanup should not affect other namespaces, got=%v", podsInOther)
	}
}

func TestCleanupDeletesStackWhenServiceIsMissing(t *testing.T) {
	repo := NewInMemoryRepository(1)
	k8s := NewMockKubernetesClient(1)
	svc := NewService(config.StackConfig{
		Namespace:         "stacks",
		StackTTL:          time.Hour,
		SchedulerInterval: time.Second,
		NodePortMin:       30000,
		NodePortMax:       30010,
	}, repo, k8s)

	st, err := svc.Create(context.Background(), CreateInput{
		TargetPorts: []PortSpec{{ContainerPort: 5000, Protocol: "TCP"}},
		PodSpecYML: `
apiVersion: v1
kind: Pod
metadata:
  name: p
spec:
  containers:
    - name: app
      image: nginx:latest
      ports:
        - containerPort: 5000
      resources:
        limits:
          cpu: "100m"
          memory: "64Mi"
`,
	})
	if err != nil {
		t.Fatalf("create error: %v", err)
	}

	k8s.mu.Lock()
	delete(k8s.services, st.ServiceName)
	k8s.mu.Unlock()

	svc.CleanupExpiredAndOrphaned(context.Background())

	if _, err := svc.GetDetails(context.Background(), st.StackID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected stack to be deleted when service is missing, got err=%v", err)
	}
}

func TestCleanupDeletesStackWhenPodIsMissing(t *testing.T) {
	repo := NewInMemoryRepository(1)
	k8s := NewMockKubernetesClient(1)
	svc := NewService(config.StackConfig{
		Namespace:         "stacks",
		StackTTL:          time.Hour,
		SchedulerInterval: time.Second,
		NodePortMin:       30000,
		NodePortMax:       30010,
	}, repo, k8s)

	st, err := svc.Create(context.Background(), CreateInput{
		TargetPorts: []PortSpec{{ContainerPort: 5000, Protocol: "TCP"}},
		PodSpecYML: `
apiVersion: v1
kind: Pod
metadata:
  name: p
spec:
  containers:
    - name: app
      image: nginx:latest
      ports:
        - containerPort: 5000
      resources:
        limits:
          cpu: "100m"
          memory: "64Mi"
`,
	})
	if err != nil {
		t.Fatalf("create error: %v", err)
	}

	k8s.mu.Lock()
	delete(k8s.pods, st.PodID)
	k8s.mu.Unlock()

	svc.CleanupExpiredAndOrphaned(context.Background())

	if _, err := svc.GetDetails(context.Background(), st.StackID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected stack to be deleted when pod is missing, got err=%v", err)
	}
}

type failingKubernetesClient struct {
	createErr error
}

func (f *failingKubernetesClient) CreatePodAndService(_ context.Context, _ ProvisionRequest) (ProvisionResult, error) {
	return ProvisionResult{}, f.createErr
}

func (f *failingKubernetesClient) DeletePodAndService(_ context.Context, _, _, _ string) error {
	return nil
}

func (f *failingKubernetesClient) GetPodStatus(_ context.Context, _, _ string) (Status, string, error) {
	return StatusRunning, "worker-a", nil
}

func (f *failingKubernetesClient) ListPods(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func (f *failingKubernetesClient) ListPodsWithCreation(_ context.Context, _ string) (map[string]PodInfo, error) {
	return map[string]PodInfo{}, nil
}

func (f *failingKubernetesClient) ListServices(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func (f *failingKubernetesClient) NodeExists(_ context.Context, _ string) (bool, error) {
	return true, nil
}

func (f *failingKubernetesClient) HasIngressNetworkPolicy(_ context.Context, _ string) (bool, error) {
	return true, nil
}

func (f *failingKubernetesClient) GetNodePublicIP(_ context.Context, _ string) (*string, error) {
	return nil, nil
}

func (f *failingKubernetesClient) CountSchedulableNodes(_ context.Context) (int, error) {
	return 0, nil
}

package stack

import (
	"context"
	"errors"
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
		TargetPort: 5000,
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

	if st.NodePort < 30000 || st.NodePort > 30010 {
		t.Fatalf("unexpected node port: %d", st.NodePort)
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
		TargetPort: 5000,
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
	}
	k8s.pods["other-ns-pod"] = podState{
		namespace: "other",
		podID:     "other-ns-pod",
		service:   "svc-other-ns-pod",
		nodeID:    "worker-a",
		status:    StatusRunning,
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
		TargetPort: 5000,
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
		TargetPort: 5000,
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

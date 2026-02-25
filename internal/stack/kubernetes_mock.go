package stack

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

type MockKubernetesClient struct {
	mu       sync.RWMutex
	rand     *rand.Rand
	nodes    map[string]bool
	nodeIPs  map[string]*string
	pods     map[string]podState
	services map[string]string
}

type podState struct {
	namespace string
	podID     string
	service   string
	nodeID    string
	status    Status
	createdAt time.Time
	stackID   string
}

func NewMockKubernetesClient(seed int64) *MockKubernetesClient {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}

	return &MockKubernetesClient{
		rand: rand.New(rand.NewSource(seed)),
		nodes: map[string]bool{
			"worker-a": true,
			"worker-b": true,
			"worker-c": true,
		},
		nodeIPs: map[string]*string{
			"worker-a": strPtr("203.0.113.10"),
			"worker-b": nil,
			"worker-c": strPtr("203.0.113.12"),
		},
		pods:     make(map[string]podState),
		services: make(map[string]string),
	}
}

func (m *MockKubernetesClient) CreatePodAndService(_ context.Context, req ProvisionRequest) (ProvisionResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	nodeID, err := m.pickNodeLocked()
	if err != nil {
		return ProvisionResult{}, err
	}

	podName := req.PodName
	if podName == "" {
		podName = req.StackID
	}

	podID := fmt.Sprintf("stack-%s", podName)
	serviceName := fmt.Sprintf("svc-%s", req.StackID)

	m.pods[podID] = podState{
		namespace: req.Namespace,
		podID:     podID,
		service:   serviceName,
		nodeID:    nodeID,
		status:    StatusRunning,
		createdAt: time.Now().UTC(),
		stackID:   req.StackID,
	}
	m.services[serviceName] = req.Namespace

	return ProvisionResult{
		PodID:       podID,
		ServiceName: serviceName,
		NodeID:      nodeID,
		Status:      StatusRunning,
	}, nil
}

func (m *MockKubernetesClient) DeletePodAndService(_ context.Context, namespace, podID, serviceName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if serviceName != "" {
		if svcNS, ok := m.services[serviceName]; ok {
			if svcNS != namespace {
				return fmt.Errorf("service namespace mismatch")
			}

			delete(m.services, serviceName)
		}
	}

	p, ok := m.pods[podID]
	if ok {
		if p.namespace != namespace {
			return fmt.Errorf("pod namespace mismatch")
		}

		delete(m.pods, podID)

		if p.service != "" {
			if svcNS, svcOK := m.services[p.service]; svcOK && svcNS == namespace {
				delete(m.services, p.service)
			}
		}
	}
	return nil
}

func (m *MockKubernetesClient) GetPodStatus(_ context.Context, namespace, podID string) (Status, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	p, ok := m.pods[podID]
	if !ok || p.namespace != namespace {
		return "", "", ErrNotFound
	}

	if !m.nodes[p.nodeID] {
		return StatusNodeDeleted, p.nodeID, nil
	}

	return p.status, p.nodeID, nil
}

func (m *MockKubernetesClient) ListPods(_ context.Context, namespace string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]string, 0)
	for _, p := range m.pods {
		if p.namespace == namespace {
			out = append(out, p.podID)
		}
	}
	return out, nil
}

func (m *MockKubernetesClient) ListPodsWithCreation(_ context.Context, namespace string) (map[string]PodInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make(map[string]PodInfo)
	for _, p := range m.pods {
		if p.namespace == namespace {
			out[p.podID] = PodInfo{
				CreatedAt: p.createdAt,
				StackID:   p.stackID,
			}
		}
	}

	return out, nil
}

func (m *MockKubernetesClient) ListServices(_ context.Context, namespace string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]string, 0)
	for svcName, svcNS := range m.services {
		if svcNS == namespace {
			out = append(out, svcName)
		}
	}

	return out, nil
}

func (m *MockKubernetesClient) NodeExists(_ context.Context, nodeID string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.nodes[nodeID], nil
}

func (m *MockKubernetesClient) HasIngressNetworkPolicy(_ context.Context) (bool, error) {
	return true, nil
}

func (m *MockKubernetesClient) GetNodePublicIP(_ context.Context, nodeID string) (*string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ip, ok := m.nodeIPs[nodeID]
	if !ok {
		return nil, nil
	}
	return ip, nil
}

func (m *MockKubernetesClient) CountSchedulableNodes(_ context.Context) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, alive := range m.nodes {
		if alive {
			count++
		}
	}

	return count, nil
}

func (m *MockKubernetesClient) pickNodeLocked() (string, error) {
	healthy := make([]string, 0)
	for id, alive := range m.nodes {
		if alive {
			healthy = append(healthy, id)
		}
	}

	if len(healthy) == 0 {
		return "", fmt.Errorf("no schedulable nodes")
	}

	return healthy[m.rand.Intn(len(healthy))], nil
}

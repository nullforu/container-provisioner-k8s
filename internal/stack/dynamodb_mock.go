package stack

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"
)

type CreateConstraints struct {
	MaxReservedCPUMilli    int64
	MaxReservedMemoryBytes int64
}

type RepositoryClientAPI interface {
	Create(ctx context.Context, st Stack, constraints CreateConstraints) error
	Get(ctx context.Context, stackID string) (Stack, bool, error)
	Delete(ctx context.Context, stackID string) (Stack, bool, error)
	ListAll(ctx context.Context) ([]Stack, error)
	ReserveNodePort(ctx context.Context, min, max int) (int, error)
	ReleaseNodePort(ctx context.Context, port int) error
	UsedNodePortCount(ctx context.Context) (int, error)
	UpdateStatus(ctx context.Context, stackID string, status Status, nodeID string) error
}

type InMemoryRepository struct {
	mu             sync.RWMutex
	stacks         map[string]Stack
	ports          map[int]string
	reservedCPU    int64
	reservedMemory int64
	rand           *rand.Rand
}

func NewInMemoryRepository(seed int64) *InMemoryRepository {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}

	return &InMemoryRepository{
		stacks: make(map[string]Stack),
		ports:  make(map[int]string),
		rand:   rand.New(rand.NewSource(seed)),
	}
}

func (r *InMemoryRepository) Create(_ context.Context, st Stack, constraints CreateConstraints) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.stacks[st.StackID]; exists {
		return fmt.Errorf("stack id already exists")
	}

	if owner, exists := r.ports[st.NodePort]; !exists || owner != "" {
		return ErrNoAvailableNodePort
	}

	if constraints.MaxReservedCPUMilli > 0 && r.reservedCPU+st.RequestedMilli >= constraints.MaxReservedCPUMilli {
		return ErrClusterSaturated
	}
	if constraints.MaxReservedMemoryBytes > 0 && r.reservedMemory+st.RequestedBytes >= constraints.MaxReservedMemoryBytes {
		return ErrClusterSaturated
	}

	r.stacks[st.StackID] = st
	r.ports[st.NodePort] = st.StackID
	r.reservedCPU += st.RequestedMilli
	r.reservedMemory += st.RequestedBytes

	return nil
}

func (r *InMemoryRepository) Get(_ context.Context, stackID string) (Stack, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	st, ok := r.stacks[stackID]
	return st, ok, nil
}

func (r *InMemoryRepository) Delete(_ context.Context, stackID string) (Stack, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	st, ok := r.stacks[stackID]
	if !ok {
		return Stack{}, false, nil
	}

	delete(r.stacks, stackID)
	delete(r.ports, st.NodePort)

	if r.reservedCPU >= st.RequestedMilli {
		r.reservedCPU -= st.RequestedMilli
	}

	if r.reservedMemory >= st.RequestedBytes {
		r.reservedMemory -= st.RequestedBytes
	}

	return st, true, nil
}

func (r *InMemoryRepository) ListAll(_ context.Context) ([]Stack, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Stack, 0, len(r.stacks))
	for _, st := range r.stacks {
		result = append(result, st)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})

	return result, nil
}

func (r *InMemoryRepository) ReserveNodePort(_ context.Context, min, max int) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	total := max - min + 1
	if total <= 0 {
		return 0, ErrNoAvailableNodePort
	}

	if len(r.ports) >= total {
		return 0, ErrNoAvailableNodePort
	}

	start := min + r.rand.Intn(total)
	for i := range total {
		port := min + ((start - min + i) % total)
		if _, exists := r.ports[port]; !exists {
			r.ports[port] = ""
			return port, nil
		}
	}

	return 0, ErrNoAvailableNodePort
}

func (r *InMemoryRepository) ReleaseNodePort(_ context.Context, port int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if owner, exists := r.ports[port]; exists && owner == "" {
		delete(r.ports, port)
	}

	return nil
}

func (r *InMemoryRepository) UsedNodePortCount(_ context.Context) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.ports), nil
}

func (r *InMemoryRepository) UpdateStatus(_ context.Context, stackID string, status Status, nodeID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	st, ok := r.stacks[stackID]
	if !ok {
		return ErrNotFound
	}

	st.Status = status
	if nodeID != "" {
		st.NodeID = nodeID
	}

	st.UpdatedAt = time.Now().UTC()
	r.stacks[stackID] = st

	return nil
}

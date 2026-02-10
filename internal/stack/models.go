package stack

import "time"

type Status string

const (
	StatusCreating    Status = "creating"
	StatusRunning     Status = "running"
	StatusStopped     Status = "stopped"
	StatusFailed      Status = "failed"
	StatusNodeDeleted Status = "node_deleted"
)

type Stack struct {
	StackID        string    `json:"stack_id"`
	PodID          string    `json:"pod_id"`
	Namespace      string    `json:"namespace"`
	NodeID         string    `json:"node_id"`
	NodePublicIP   *string   `json:"node_public_ip"`
	PodSpecYAML    string    `json:"pod_spec"`
	TargetPort     int       `json:"target_port"`
	NodePort       int       `json:"node_port"`
	ServiceName    string    `json:"service_name"`
	Status         Status    `json:"status"`
	TTLExpiresAt   time.Time `json:"ttl_expires_at"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	RequestedMilli int64     `json:"requested_cpu_milli"`
	RequestedBytes int64     `json:"requested_memory_bytes"`
}

type CreateInput struct {
	PodSpecYML string
	TargetPort int
}

type Stats struct {
	TotalStacks         int            `json:"total_stacks"`
	ActiveStacks        int            `json:"active_stacks"`
	NodeDistribution    map[string]int `json:"node_distribution"`
	UsedNodePorts       int            `json:"used_node_ports"`
	ReservedCPUMilli    int64          `json:"reserved_cpu_milli"`
	ReservedMemoryBytes int64          `json:"reserved_memory_bytes"`
}

type StackStatusSummary struct {
	StackID      string    `json:"stack_id"`
	Status       Status    `json:"status"`
	TTL          time.Time `json:"ttl"`
	NodePort     int       `json:"node_port"`
	TargetPort   int       `json:"target_port"`
	NodePublicIP *string   `json:"node_public_ip"`
}

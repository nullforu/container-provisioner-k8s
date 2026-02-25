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
	StackID        string        `json:"stack_id"`
	PodID          string        `json:"pod_id"`
	Namespace      string        `json:"namespace"`
	NodeID         string        `json:"node_id"`
	NodePublicIP   *string       `json:"node_public_ip"`
	PodSpecYAML    string        `json:"pod_spec"`
	TargetPorts    []PortSpec    `json:"-"`
	Ports          []PortMapping `json:"ports"`
	ServiceName    string        `json:"service_name"`
	Status         Status        `json:"status"`
	TTLExpiresAt   time.Time     `json:"ttl_expires_at"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
	RequestedMilli int64         `json:"requested_cpu_milli"`
	RequestedBytes int64         `json:"requested_memory_bytes"`
}

type PortSpec struct {
	ContainerPort int    `json:"container_port"`
	Protocol      string `json:"protocol"`
}

type PortMapping struct {
	ContainerPort int    `json:"container_port"`
	Protocol      string `json:"protocol"`
	NodePort      int    `json:"node_port"`
}

type CreateInput struct {
	PodSpecYML  string
	TargetPorts []PortSpec
}

type JobStatus string

const (
	JobStatusQueued    JobStatus = "queued"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
)

type JobError struct {
	StackID string `json:"stack_id"`
	Error   string `json:"error"`
}

type BatchDeleteJob struct {
	JobID     string     `json:"job_id"`
	Status    JobStatus  `json:"status"`
	Total     int        `json:"total"`
	Deleted   int        `json:"deleted"`
	NotFound  int        `json:"not_found"`
	Failed    int        `json:"failed"`
	Errors    []JobError `json:"errors,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
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
	StackID      string        `json:"stack_id"`
	Status       Status        `json:"status"`
	TTL          time.Time     `json:"ttl"`
	TargetPorts  []PortSpec    `json:"-"`
	Ports        []PortMapping `json:"ports"`
	NodePublicIP *string       `json:"node_public_ip"`
}

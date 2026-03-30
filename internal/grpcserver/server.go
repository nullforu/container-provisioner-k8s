package grpcserver

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	stackv1 "smctf/internal/gen/stack/v1"
	"smctf/internal/stack"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type StackService interface {
	Create(ctx context.Context, in stack.CreateInput) (stack.Stack, error)
	GetDetails(ctx context.Context, stackID string) (stack.Stack, error)
	GetStatusSummary(ctx context.Context, stackID string) (stack.StackStatusSummary, error)
	Delete(ctx context.Context, stackID string) error
	ListAll(ctx context.Context) ([]stack.Stack, error)
	StartBatchDelete(ctx context.Context, stackIDs []string) (string, error)
	GetBatchDeleteJob(ctx context.Context, jobID string) (stack.BatchDeleteJob, error)
	Stats(ctx context.Context) (stack.Stats, error)
}

type Server struct {
	stackv1.UnimplementedStackServiceServer
	service StackService
	logger  *slog.Logger
}

func New(service StackService, logger *slog.Logger) *Server {
	return &Server{service: service, logger: logger}
}

func (s *Server) Healthz(_ context.Context, _ *stackv1.HealthzRequest) (*stackv1.HealthzResponse, error) {
	return &stackv1.HealthzResponse{Status: "ok"}, nil
}

func (s *Server) CreateStack(ctx context.Context, req *stackv1.CreateStackRequest) (*stackv1.CreateStackResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	input := stack.CreateInput{
		PodSpecYML:  strings.TrimSpace(req.PodSpec),
		TargetPorts: fromProtoPortSpecs(req.TargetPorts),
	}

	st, err := s.service.Create(ctx, input)
	if err != nil {
		return nil, s.grpcError(err)
	}

	return &stackv1.CreateStackResponse{Stack: toProtoStack(st)}, nil
}

func (s *Server) GetStack(ctx context.Context, req *stackv1.GetStackRequest) (*stackv1.GetStackResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	stackID := strings.TrimSpace(req.GetStackId())
	if stackID == "" {
		return nil, status.Error(codes.InvalidArgument, "stack_id is required")
	}

	st, err := s.service.GetDetails(ctx, stackID)
	if err != nil {
		return nil, s.grpcError(err)
	}

	return &stackv1.GetStackResponse{Stack: toProtoStack(st)}, nil
}

func (s *Server) GetStackStatusSummary(ctx context.Context, req *stackv1.GetStackStatusSummaryRequest) (*stackv1.GetStackStatusSummaryResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	stackID := strings.TrimSpace(req.GetStackId())
	if stackID == "" {
		return nil, status.Error(codes.InvalidArgument, "stack_id is required")
	}

	summary, err := s.service.GetStatusSummary(ctx, stackID)
	if err != nil {
		return nil, s.grpcError(err)
	}

	return &stackv1.GetStackStatusSummaryResponse{Summary: toProtoStackStatusSummary(summary)}, nil
}

func (s *Server) DeleteStack(ctx context.Context, req *stackv1.DeleteStackRequest) (*stackv1.DeleteStackResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	stackID := strings.TrimSpace(req.GetStackId())
	if stackID == "" {
		return nil, status.Error(codes.InvalidArgument, "stack_id is required")
	}

	if err := s.service.Delete(ctx, stackID); err != nil {
		return nil, s.grpcError(err)
	}

	return &stackv1.DeleteStackResponse{Deleted: true, StackId: stackID}, nil
}

func (s *Server) ListStacks(ctx context.Context, _ *stackv1.ListStacksRequest) (*stackv1.ListStacksResponse, error) {
	items, err := s.service.ListAll(ctx)
	if err != nil {
		return nil, s.grpcError(err)
	}

	out := make([]*stackv1.Stack, 0, len(items))
	for _, item := range items {
		out = append(out, toProtoStack(item))
	}

	return &stackv1.ListStacksResponse{Stacks: out}, nil
}

func (s *Server) CreateBatchDeleteJob(ctx context.Context, req *stackv1.CreateBatchDeleteJobRequest) (*stackv1.CreateBatchDeleteJobResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	clean := make([]string, 0, len(req.StackIds))
	seen := make(map[string]struct{}, len(req.StackIds))
	for _, id := range req.StackIds {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, status.Error(codes.InvalidArgument, "stack_ids must not contain empty values")
		}

		if _, ok := seen[id]; ok {
			continue
		}

		seen[id] = struct{}{}
		clean = append(clean, id)
	}

	jobID, err := s.service.StartBatchDelete(ctx, clean)
	if err != nil {
		return nil, s.grpcError(err)
	}

	return &stackv1.CreateBatchDeleteJobResponse{JobId: jobID}, nil
}

func (s *Server) GetBatchDeleteJob(ctx context.Context, req *stackv1.GetBatchDeleteJobRequest) (*stackv1.GetBatchDeleteJobResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	jobID := strings.TrimSpace(req.GetJobId())
	if jobID == "" {
		return nil, status.Error(codes.InvalidArgument, "job_id is required")
	}

	job, err := s.service.GetBatchDeleteJob(ctx, jobID)
	if err != nil {
		return nil, s.grpcError(err)
	}

	return &stackv1.GetBatchDeleteJobResponse{Job: toProtoBatchDeleteJob(job)}, nil
}

func (s *Server) GetStats(ctx context.Context, _ *stackv1.GetStatsRequest) (*stackv1.GetStatsResponse, error) {
	stats, err := s.service.Stats(ctx)
	if err != nil {
		return nil, s.grpcError(err)
	}

	return &stackv1.GetStatsResponse{Stats: toProtoStats(stats)}, nil
}

func (s *Server) grpcError(err error) error {
	switch {
	case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	case errors.Is(err, stack.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, stack.ErrInvalidInput), errors.Is(err, stack.ErrPodSpecInvalid):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, stack.ErrNoAvailableNodePort), errors.Is(err, stack.ErrClusterSaturated):
		return status.Error(codes.Unavailable, err.Error())
	default:
		if s.logger != nil {
			s.logger.Error("grpc internal error", slog.Any("error", err))
		}
		return status.Error(codes.Internal, "internal server error")
	}
}

func toProtoStack(st stack.Stack) *stackv1.Stack {
	pb := &stackv1.Stack{
		StackId:              st.StackID,
		PodId:                st.PodID,
		Namespace:            st.Namespace,
		NodeId:               st.NodeID,
		PodSpec:              st.PodSpecYAML,
		Ports:                toProtoPortMappings(st.Ports),
		ServiceName:          st.ServiceName,
		Status:               toProtoStatus(st.Status),
		TtlExpiresAt:         tsOrNil(st.TTLExpiresAt),
		CreatedAt:            tsOrNil(st.CreatedAt),
		UpdatedAt:            tsOrNil(st.UpdatedAt),
		RequestedCpuMilli:    st.RequestedMilli,
		RequestedMemoryBytes: st.RequestedBytes,
		TargetPorts:          toProtoPortSpecs(st.TargetPorts),
	}
	if st.NodePublicIP != nil {
		pb.NodePublicIp = st.NodePublicIP
	}

	return pb
}

func toProtoStackStatusSummary(summary stack.StackStatusSummary) *stackv1.StackStatusSummary {
	pb := &stackv1.StackStatusSummary{
		StackId:     summary.StackID,
		Status:      toProtoStatus(summary.Status),
		Ttl:         tsOrNil(summary.TTL),
		Ports:       toProtoPortMappings(summary.Ports),
		TargetPorts: toProtoPortSpecs(summary.TargetPorts),
	}
	if summary.NodePublicIP != nil {
		pb.NodePublicIp = summary.NodePublicIP
	}

	return pb
}

func toProtoStats(stats stack.Stats) *stackv1.Stats {
	nodes := make(map[string]int32, len(stats.NodeDistribution))
	for key, value := range stats.NodeDistribution {
		nodes[key] = int32(value)
	}

	return &stackv1.Stats{
		TotalStacks:         int32(stats.TotalStacks),
		ActiveStacks:        int32(stats.ActiveStacks),
		NodeDistribution:    nodes,
		UsedNodePorts:       int32(stats.UsedNodePorts),
		ReservedCpuMilli:    stats.ReservedCPUMilli,
		ReservedMemoryBytes: stats.ReservedMemoryBytes,
	}
}

func toProtoBatchDeleteJob(job stack.BatchDeleteJob) *stackv1.BatchDeleteJob {
	errorsOut := make([]*stackv1.JobError, 0, len(job.Errors))
	for _, errItem := range job.Errors {
		errorsOut = append(errorsOut, &stackv1.JobError{
			StackId: errItem.StackID,
			Error:   errItem.Error,
		})
	}

	return &stackv1.BatchDeleteJob{
		JobId:     job.JobID,
		Status:    toProtoJobStatus(job.Status),
		Total:     int32(job.Total),
		Deleted:   int32(job.Deleted),
		NotFound:  int32(job.NotFound),
		Failed:    int32(job.Failed),
		Errors:    errorsOut,
		CreatedAt: tsOrNil(job.CreatedAt),
		UpdatedAt: tsOrNil(job.UpdatedAt),
	}
}

func toProtoPortSpecs(specs []stack.PortSpec) []*stackv1.PortSpec {
	out := make([]*stackv1.PortSpec, 0, len(specs))
	for _, spec := range specs {
		out = append(out, &stackv1.PortSpec{
			ContainerPort: int32(spec.ContainerPort),
			Protocol:      spec.Protocol,
		})
	}

	return out
}

func fromProtoPortSpecs(specs []*stackv1.PortSpec) []stack.PortSpec {
	out := make([]stack.PortSpec, 0, len(specs))
	for _, spec := range specs {
		if spec == nil {
			continue
		}
		out = append(out, stack.PortSpec{
			ContainerPort: int(spec.ContainerPort),
			Protocol:      spec.Protocol,
		})
	}

	return out
}

func toProtoPortMappings(mappings []stack.PortMapping) []*stackv1.PortMapping {
	out := make([]*stackv1.PortMapping, 0, len(mappings))
	for _, mapping := range mappings {
		out = append(out, &stackv1.PortMapping{
			ContainerPort: int32(mapping.ContainerPort),
			Protocol:      mapping.Protocol,
			NodePort:      int32(mapping.NodePort),
		})
	}

	return out
}

func toProtoStatus(statusVal stack.Status) stackv1.Status {
	switch statusVal {
	case stack.StatusCreating:
		return stackv1.Status_STATUS_CREATING
	case stack.StatusRunning:
		return stackv1.Status_STATUS_RUNNING
	case stack.StatusStopped:
		return stackv1.Status_STATUS_STOPPED
	case stack.StatusFailed:
		return stackv1.Status_STATUS_FAILED
	case stack.StatusNodeDeleted:
		return stackv1.Status_STATUS_NODE_DELETED
	default:
		return stackv1.Status_STATUS_UNSPECIFIED
	}
}

func toProtoJobStatus(statusVal stack.JobStatus) stackv1.JobStatus {
	switch statusVal {
	case stack.JobStatusQueued:
		return stackv1.JobStatus_JOB_STATUS_QUEUED
	case stack.JobStatusRunning:
		return stackv1.JobStatus_JOB_STATUS_RUNNING
	case stack.JobStatusCompleted:
		return stackv1.JobStatus_JOB_STATUS_COMPLETED
	case stack.JobStatusFailed:
		return stackv1.JobStatus_JOB_STATUS_FAILED
	default:
		return stackv1.JobStatus_JOB_STATUS_UNSPECIFIED
	}
}

func tsOrNil(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}

	return timestamppb.New(t)
}

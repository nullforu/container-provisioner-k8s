package grpcserver

import (
	"context"
	"net"
	"testing"
	"time"

	"smctf/internal/config"
	stackv1 "smctf/internal/gen/stack/v1"
	"smctf/internal/stack"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

func dialTestServer(t *testing.T, svc StackService, apiKey config.APIKeyConfig) (*grpc.ClientConn, func()) {
	listener := bufconn.Listen(bufSize)
	server := grpc.NewServer(grpc.UnaryInterceptor(APIKeyUnaryInterceptor(apiKey)))
	stackv1.RegisterStackServiceServer(server, New(svc, nil))

	go func() {
		_ = server.Serve(listener)
	}()

	conn, err := grpc.NewClient("passthrough:///bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		listener.Close()
		server.Stop()
		t.Fatalf("dial: %v", err)
	}

	cleanup := func() {
		conn.Close()
		server.Stop()
		listener.Close()
	}

	return conn, cleanup
}

type stubStackService struct {
	createFn            func(context.Context, stack.CreateInput) (stack.Stack, error)
	getDetailsFn        func(context.Context, string) (stack.Stack, error)
	getStatusSummaryFn  func(context.Context, string) (stack.StackStatusSummary, error)
	deleteFn            func(context.Context, string) error
	listAllFn           func(context.Context) ([]stack.Stack, error)
	startBatchDeleteFn  func(context.Context, []string) (string, error)
	getBatchDeleteJobFn func(context.Context, string) (stack.BatchDeleteJob, error)
	statsFn             func(context.Context) (stack.Stats, error)
}

func (s stubStackService) Create(ctx context.Context, in stack.CreateInput) (stack.Stack, error) {
	if s.createFn != nil {
		return s.createFn(ctx, in)
	}

	return stack.Stack{}, nil
}

func (s stubStackService) GetDetails(ctx context.Context, stackID string) (stack.Stack, error) {
	if s.getDetailsFn != nil {
		return s.getDetailsFn(ctx, stackID)
	}

	return stack.Stack{}, nil
}

func (s stubStackService) GetStatusSummary(ctx context.Context, stackID string) (stack.StackStatusSummary, error) {
	if s.getStatusSummaryFn != nil {
		return s.getStatusSummaryFn(ctx, stackID)
	}

	return stack.StackStatusSummary{}, nil
}

func (s stubStackService) Delete(ctx context.Context, stackID string) error {
	if s.deleteFn != nil {
		return s.deleteFn(ctx, stackID)
	}

	return nil
}

func (s stubStackService) ListAll(ctx context.Context) ([]stack.Stack, error) {
	if s.listAllFn != nil {
		return s.listAllFn(ctx)
	}

	return nil, nil
}

func (s stubStackService) StartBatchDelete(ctx context.Context, stackIDs []string) (string, error) {
	if s.startBatchDeleteFn != nil {
		return s.startBatchDeleteFn(ctx, stackIDs)
	}

	return "", nil
}

func (s stubStackService) GetBatchDeleteJob(ctx context.Context, jobID string) (stack.BatchDeleteJob, error) {
	if s.getBatchDeleteJobFn != nil {
		return s.getBatchDeleteJobFn(ctx, jobID)
	}

	return stack.BatchDeleteJob{}, nil
}

func (s stubStackService) Stats(ctx context.Context) (stack.Stats, error) {
	if s.statsFn != nil {
		return s.statsFn(ctx)
	}

	return stack.Stats{}, nil
}

func TestHealthz(t *testing.T) {
	conn, cleanup := dialTestServer(t, stubStackService{}, config.APIKeyConfig{Enabled: false})
	defer cleanup()

	client := stackv1.NewStackServiceClient(conn)
	resp, err := client.Healthz(context.Background(), &stackv1.HealthzRequest{})
	if err != nil {
		t.Fatalf("healthz: %v", err)
	}

	if resp.GetStatus() != "ok" {
		t.Fatalf("expected ok, got %q", resp.GetStatus())
	}
}

func TestAuthInterceptor(t *testing.T) {
	conn, cleanup := dialTestServer(t, stubStackService{}, config.APIKeyConfig{Enabled: true, Value: "secret"})
	defer cleanup()

	client := stackv1.NewStackServiceClient(conn)
	_, err := client.Healthz(context.Background(), &stackv1.HealthzRequest{})
	if err == nil {
		t.Fatalf("expected unauthenticated error")
	}

	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Fatalf("expected unauthenticated, got %v", st.Code())
	}

	ctx := metadata.AppendToOutgoingContext(context.Background(), "x-api-key", "secret")
	resp, err := client.Healthz(ctx, &stackv1.HealthzRequest{})
	if err != nil {
		t.Fatalf("healthz with api key: %v", err)
	}

	if resp.GetStatus() != "ok" {
		t.Fatalf("expected ok, got %q", resp.GetStatus())
	}
}

func TestGetStats(t *testing.T) {
	service := stubStackService{
		statsFn: func(context.Context) (stack.Stats, error) {
			return stack.Stats{
				TotalStacks:         3,
				ActiveStacks:        2,
				NodeDistribution:    map[string]int{"node-a": 2, "node-b": 1},
				UsedNodePorts:       3,
				ReservedCPUMilli:    500,
				ReservedMemoryBytes: 256,
			}, nil
		},
	}

	conn, cleanup := dialTestServer(t, service, config.APIKeyConfig{Enabled: false})
	defer cleanup()

	client := stackv1.NewStackServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := client.GetStats(ctx, &stackv1.GetStatsRequest{})
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}

	if resp.GetStats().GetTotalStacks() != 3 || resp.GetStats().GetActiveStacks() != 2 {
		t.Fatalf("unexpected stats response: %+v", resp.GetStats())
	}
}

//go:build integration

package tracing_test

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	tracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	otlptracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/protobuf/types/known/emptypb"

	otel "github.com/beabys/otelpkg"
	tracing "github.com/beabys/otelpkg/tracing"
)

// fakeCollector implements the OTLP TraceServiceServer and stores all
// received resource spans in memory for test assertions.
type fakeCollector struct {
	tracev1.UnimplementedTraceServiceServer

	mu    sync.Mutex
	spans []*otlptracepb.Span
}

func (c *fakeCollector) Export(_ context.Context, req *tracev1.ExportTraceServiceRequest) (*tracev1.ExportTraceServiceResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, rs := range req.GetResourceSpans() {
		for _, ss := range rs.GetScopeSpans() {
			c.spans = append(c.spans, ss.GetSpans()...)
		}
	}
	return &tracev1.ExportTraceServiceResponse{}, nil
}

func (c *fakeCollector) spanNames() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	names := make([]string, len(c.spans))
	for i, s := range c.spans {
		names[i] = s.GetName()
	}
	return names
}

// authServer is the interface type required by grpc.ServiceDesc.HandlerType.
// authStub satisfies this (empty) interface.
type authServer interface{}

// authStub is a minimal gRPC server implementation that registers under
// the service name "auth.v1.AuthService" with a single unary RPC "Login".
type authStub struct{}

func authStubLoginHandler(_ interface{}, ctx context.Context, dec func(interface{}) error, _ grpc.UnaryServerInterceptor) (interface{}, error) {
	req := &emptypb.Empty{}
	if err := dec(req); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

var authServiceDesc = grpc.ServiceDesc{
	ServiceName: "auth.v1.AuthService",
	HandlerType: (*authServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Login",
			Handler:    authStubLoginHandler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "auth/v1/auth.proto",
}

func startFakeCollector(t *testing.T) (*fakeCollector, string) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("collector listen: %v", err)
	}
	collector := &fakeCollector{}
	srv := grpc.NewServer()
	tracev1.RegisterTraceServiceServer(srv, collector)
	go srv.Serve(lis)
	t.Cleanup(srv.GracefulStop)
	return collector, lis.Addr().String()
}

func TestIntegration_TracingEndToEnd(t *testing.T) {
	// --- 1. Start fake OTLP collector ---
	collector, collectorAddr := startFakeCollector(t)

	// --- 2. Init tracing ---
	shutdown, err := tracing.Init(tracing.Config{
		Enabled:     true,
		Endpoint:    collectorAddr,
		ServiceName: "auth-test",
		SampleRatio: 1.0,
	})
	if err != nil {
		t.Fatalf("tracing.Init: %v", err)
	}

	// --- 3. Start gRPC server with tracing handler ---
	skipMethods := []string{"/grpc.health.v1.Health/Check"}
	handler := otel.NewGRPCServerHandler(skipMethods)

	grpcSrv := grpc.NewServer(grpc.StatsHandler(handler))

	// Register auth stub
	grpcSrv.RegisterService(&authServiceDesc, &authStub{})

	// Register health service
	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(grpcSrv, healthSrv)
	healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	grpcLis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grpc listen: %v", err)
	}
	go grpcSrv.Serve(grpcLis)
	t.Cleanup(grpcSrv.GracefulStop)

	// --- 4. Create gRPC client ---
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, grpcLis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	// --- 5. RPC: /auth.v1.AuthService/Login ---
	loginReq := &emptypb.Empty{}
	loginResp := &emptypb.Empty{}
	err = conn.Invoke(ctx, "/auth.v1.AuthService/Login", loginReq, loginResp)
	if err != nil {
		t.Fatalf("Login RPC: %v", err)
	}

	// --- 6. RPC: /grpc.health.v1.Health/Check (skipped) ---
	healthClient := healthpb.NewHealthClient(conn)
	_, err = healthClient.Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("Health Check RPC: %v", err)
	}

	// --- 7. Flush tracer ---
	if err := shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	// --- 8. Assert spans ---
	// Give the batch exporter a moment to flush to collector
	time.Sleep(500 * time.Millisecond)

	names := collector.spanNames()

	// Login span MUST exist
	foundLogin := false
	for _, n := range names {
		if n == "auth.v1.AuthService/Login" {
			foundLogin = true
			break
		}
	}
	if !foundLogin {
		t.Errorf("expected span 'auth.v1.AuthService/Login' not found; got spans: %v", names)
	}

	// Health Check span MUST NOT exist
	for _, n := range names {
		if n == "grpc.health.v1.Health/Check" {
			t.Errorf("skipped method 'grpc.health.v1.Health/Check' produced a span; should have been filtered")
		}
	}
}

func TestIntegration_TracingDisabled_NoDial(t *testing.T) {
	shutdown, err := tracing.Init(tracing.Config{
		Enabled:     false,
		Endpoint:    "", // no endpoint — should not matter
		ServiceName: "", // no service name — should not matter
		SampleRatio: 0,
	})
	if err != nil {
		t.Fatalf("Init with disabled should not error: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown of disabled tracing should not error: %v", err)
	}
}

func TestIntegration_TracingEnabled_EmptyEndpoint_Error(t *testing.T) {
	_, err := tracing.Init(tracing.Config{
		Enabled:     true,
		Endpoint:    "",
		ServiceName: "auth-test",
		SampleRatio: 1.0,
	})
	if err == nil {
		t.Fatal("expected error when endpoint is empty and tracing enabled")
	}
}

func TestIntegration_TracingEnabled_EmptyServiceName_Error(t *testing.T) {
	_, err := tracing.Init(tracing.Config{
		Enabled:     true,
		Endpoint:    "127.0.0.1:4317",
		ServiceName: "",
		SampleRatio: 1.0,
	})
	if err == nil {
		t.Fatal("expected error when service_name is empty and tracing enabled")
	}
}

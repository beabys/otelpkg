package otelpkg

import (
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc/stats"
)

// NewGRPCServerHandler returns a gRPC server stats.Handler that creates one
// server span per RPC, skipping full method names in skipMethods.
// The exact-match filter set is built once at construction time.
func NewGRPCServerHandler(skipMethods []string) stats.Handler {
	pf := newPathFilter(skipMethods)
	return otelgrpc.NewServerHandler(
		otelgrpc.WithFilter(func(info *stats.RPCTagInfo) bool {
			return pf.shouldTrace(info.FullMethodName)
		}),
	)
}

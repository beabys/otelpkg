package otelpkg

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGRPCServerHandler_ReturnsNonNil(t *testing.T) {
	h := NewGRPCServerHandler(nil)
	require.NotNil(t, h)
}

func TestGRPCFilter_NormalMethodTraced(t *testing.T) {
	pf := newPathFilter([]string{
		"/grpc.health.v1.Health/Check",
		"/grpc.health.v1.Health/Watch",
	})
	assert.True(t, pf.shouldTrace("/auth.v1.AuthService/Login"))
}

func TestGRPCFilter_SkippedMethodNotTraced(t *testing.T) {
	pf := newPathFilter([]string{
		"/grpc.health.v1.Health/Check",
		"/grpc.health.v1.Health/Watch",
		"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo",
	})
	assert.False(t, pf.shouldTrace("/grpc.health.v1.Health/Check"))
	assert.False(t, pf.shouldTrace("/grpc.health.v1.Health/Watch"))
	assert.False(t, pf.shouldTrace("/grpc.reflection.v1.ServerReflection/ServerReflectionInfo"))
}

func TestGRPCFilter_EmptySkipListTracesAll(t *testing.T) {
	pf := newPathFilter(nil)
	assert.True(t, pf.shouldTrace("/auth.v1.AuthService/Login"))
	assert.True(t, pf.shouldTrace("/grpc.health.v1.Health/Check"))
	assert.True(t, pf.shouldTrace("/grpc.reflection.v1.ServerReflection/ServerReflectionInfo"))
}

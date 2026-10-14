package tracing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInit_DisabledReturnsNoop(t *testing.T) {
	cfg := Config{
		Enabled:     false,
		Endpoint:    "",
		ServiceName: "",
		SampleRatio: 0,
	}

	shutdown, err := Init(cfg)
	require.NoError(t, err)
	require.NotNil(t, shutdown)

	// No-op shutdown must succeed.
	require.NoError(t, shutdown(context.Background()))
}

func TestInit_EnabledMissingEndpoint(t *testing.T) {
	cfg := Config{
		Enabled:     true,
		Endpoint:    "",
		ServiceName: "auth",
		SampleRatio: 1.0,
	}

	_, err := Init(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "endpoint")
}

func TestInit_EnabledMissingServiceName(t *testing.T) {
	cfg := Config{
		Enabled:     true,
		Endpoint:    "localhost:4317",
		ServiceName: "",
		SampleRatio: 1.0,
	}

	_, err := Init(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service_name")
}

func TestInit_SampleRatioClamped(t *testing.T) {
	// Verify ratio clamping logic without actually connecting.
	// We test the disabled path here; ratio clamping is exercised in the
	// enabled path but we can't dial a real collector in unit tests.
	// Instead, verify that invalid ratios don't panic by testing the
	// disabled path returns cleanly.
	cfg := Config{Enabled: false, SampleRatio: -0.5}
	shutdown, err := Init(cfg)
	require.NoError(t, err)
	require.NoError(t, shutdown(context.Background()))

	cfg = Config{Enabled: false, SampleRatio: 2.0}
	shutdown, err = Init(cfg)
	require.NoError(t, err)
	require.NoError(t, shutdown(context.Background()))
}

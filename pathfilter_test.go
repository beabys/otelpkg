package otelpkg

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPathFilter_EmptyListTracesAll(t *testing.T) {
	f := newPathFilter(nil)
	assert.True(t, f.shouldTrace("/health"))
	assert.True(t, f.shouldTrace("/v1/roles"))
	assert.True(t, f.shouldTrace("/"))
}

func TestPathFilter_EmptySliceTracesAll(t *testing.T) {
	f := newPathFilter([]string{})
	assert.True(t, f.shouldTrace("/health"))
	assert.True(t, f.shouldTrace("/v1/roles"))
}

func TestPathFilter_ExcludesConfiguredPaths(t *testing.T) {
	f := newPathFilter([]string{"/health", "/ready"})

	assert.False(t, f.shouldTrace("/health"), "/health should be skipped")
	assert.False(t, f.shouldTrace("/ready"), "/ready should be skipped")
	assert.True(t, f.shouldTrace("/v1/roles"), "/v1/roles should be traced")
	assert.True(t, f.shouldTrace("/"), "/ should be traced")
}

func TestPathFilter_ExactMatchOnly(t *testing.T) {
	f := newPathFilter([]string{"/health"})

	assert.False(t, f.shouldTrace("/health"))
	assert.True(t, f.shouldTrace("/health/"), "/health/ is different from /health")
	assert.True(t, f.shouldTrace("/healthz"), "/healthz is different from /health")
	assert.True(t, f.shouldTrace("/api/health"), "/api/health is different from /health")
}

func TestPathFilter_NilAndEmptyEntriesDoNotCrash(t *testing.T) {
	// Empty string in skip list should not crash
	f := newPathFilter([]string{"", "/health"})
	assert.False(t, f.shouldTrace("/health"))
	assert.True(t, f.shouldTrace("/v1/roles"))

	// Empty string path should match empty string in skip list
	assert.False(t, f.shouldTrace(""))
}

func TestPathFilter_DuplicateEntries(t *testing.T) {
	f := newPathFilter([]string{"/health", "/health", "/ready"})
	assert.False(t, f.shouldTrace("/health"))
	assert.False(t, f.shouldTrace("/ready"))
	assert.True(t, f.shouldTrace("/v1/roles"))
}

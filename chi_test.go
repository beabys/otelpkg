package otelpkg

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// setupTracing creates an in-memory span exporter and tracer provider for testing.
// Sets as global provider since otelchi uses otel.GetTracerProvider() when none configured.
// Restores prior provider on test cleanup to avoid cross-test pollution.
func setupTracing(t *testing.T) (*tracetest.SpanRecorder, *sdktrace.TracerProvider) {
	t.Helper()
	prior := otel.GetTracerProvider()
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(recorder),
	)
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(prior)
	})
	return recorder, tp
}

// TestNewChiMiddleware_ReturnsNonNil verifies the middleware factory returns non-nil
// for both nil and non-empty skipPaths.
func TestNewChiMiddleware_ReturnsNonNil(t *testing.T) {
	t.Run("nil skipPaths", func(t *testing.T) {
		mw := NewChiMiddleware("test-service", nil)
		require.NotNil(t, mw)
	})

	t.Run("empty skipPaths", func(t *testing.T) {
		mw := NewChiMiddleware("test-service", []string{})
		require.NotNil(t, mw)
	})

	t.Run("non-empty skipPaths", func(t *testing.T) {
		mw := NewChiMiddleware("test-service", []string{"/health", "/ready"})
		require.NotNil(t, mw)
	})
}

// TestChiMiddleware_NonSkippedRequest_CreatesSpan verifies that a non-skipped request
// creates exactly one span with Kind=Server and recorded status.
func TestChiMiddleware_NonSkippedRequest_CreatesSpan(t *testing.T) {
	recorder, tp := setupTracing(t)
	defer tp.Shutdown(context.Background())

	r := chi.NewRouter()
	r.Use(NewChiMiddleware("test-service", []string{"/health"}))
	r.Get("/api/users", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	spans := recorder.Ended()
	require.Len(t, spans, 1, "expected exactly one span for non-skipped request")

	span := spans[0]
	assert.Equal(t, trace.SpanKindServer, span.SpanKind(), "span kind should be Server")
	assert.True(t, span.SpanContext().IsValid(), "span should be recorded")
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestChiMiddleware_SkippedRequest_NoSpan verifies that a skipped request creates
// zero spans but the wrapped handler still executes.
func TestChiMiddleware_SkippedRequest_NoSpan(t *testing.T) {
	recorder, tp := setupTracing(t)
	defer tp.Shutdown(context.Background())

	handlerCalled := false
	r := chi.NewRouter()
	r.Use(NewChiMiddleware("test-service", []string{"/health"}))
	r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	spans := recorder.Ended()
	assert.Empty(t, spans, "expected zero spans for skipped request")
	assert.True(t, handlerCalled, "wrapped handler should still execute")
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestChiMiddleware_NilSkipPaths_TracesAll verifies that nil skipPaths traces all paths.
func TestChiMiddleware_NilSkipPaths_TracesAll(t *testing.T) {
	recorder, tp := setupTracing(t)
	defer tp.Shutdown(context.Background())

	r := chi.NewRouter()
	r.Use(NewChiMiddleware("test-service", nil))
	r.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	paths := []string{"/health", "/api/users", "/ready", "/"}
	for _, path := range paths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
	}

	spans := recorder.Ended()
	assert.Len(t, spans, len(paths), "all paths should be traced with nil skipPaths")
}

// TestChiMiddleware_EmptySkipPaths_TracesAll verifies that empty skipPaths traces all paths.
func TestChiMiddleware_EmptySkipPaths_TracesAll(t *testing.T) {
	recorder, tp := setupTracing(t)
	defer tp.Shutdown(context.Background())

	r := chi.NewRouter()
	r.Use(NewChiMiddleware("test-service", []string{}))
	r.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	paths := []string{"/health", "/api/users", "/ready"}
	for _, path := range paths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
	}

	spans := recorder.Ended()
	assert.Len(t, spans, len(paths), "all paths should be traced with empty skipPaths")
}

// TestChiMiddleware_ExactMatchSemantics verifies that skip paths use exact match
// (e.g., /health skips only /health, not /health/ or /healthz).
func TestChiMiddleware_ExactMatchSemantics(t *testing.T) {
	tests := []struct {
		path        string
		expectSpan  bool
		description string
	}{
		{"/health", false, "exact match should skip"},
		{"/health/", true, "/health/ is different from /health"},
		{"/healthz", true, "/healthz is different from /health"},
		{"/api/health", true, "/api/health is different from /health"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			// Fresh recorder per subtest for isolation
			rec, tp := setupTracing(t)
			defer tp.Shutdown(context.Background())

			r := chi.NewRouter()
			r.Use(NewChiMiddleware("test-service", []string{"/health"}))
			r.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			spans := rec.Ended()
			if tt.expectSpan {
				assert.Len(t, spans, 1, tt.description)
			} else {
				assert.Empty(t, spans, tt.description)
			}
		})
	}
}

// TestChiMiddleware_WithChiRouter_SpanName verifies that when using a real chi router,
// the span name reflects the route pattern (e.g., /users/{id}).
func TestChiMiddleware_WithChiRouter_SpanName(t *testing.T) {
	recorder, tp := setupTracing(t)
	defer tp.Shutdown(context.Background())

	r := chi.NewRouter()
	r.Use(NewChiMiddleware("test-service", nil))

	r.Get("/users/{id}", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/users/123", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	spans := recorder.Ended()
	require.Len(t, spans, 1, "expected one span")

	span := spans[0]
	// VERIFIED: otelchi v0.12.3 span name = route pattern only (no method prefix by default).
	// Observed: "/users/{id}", not "GET /users/{id}".
	assert.Equal(t, "/users/{id}", span.Name(), "span name should be route pattern")
	assert.Equal(t, trace.SpanKindServer, span.SpanKind())

	// Verify HTTP route attribute is set
	routeAttr := getAttribute(span.Attributes(), "http.route")
	require.NotNil(t, routeAttr, "http.route attribute should be set")
	assert.Equal(t, "/users/{id}", routeAttr.Value.AsString())
}

// TestChiMiddleware_PlainHandler_ViaCatchAll verifies span creation when middleware
// wraps a handler served through a chi router with a catch-all route.
// NOTE: otelchi v0.12.3 panics if the middleware runs without a chi route context
// (chi.RouteContext returns nil). Plain http.Handler usage is unsupported; always
// mount via chi router.
func TestChiMiddleware_PlainHandler_ViaCatchAll(t *testing.T) {
	recorder, tp := setupTracing(t)
	defer tp.Shutdown(context.Background())

	r := chi.NewRouter()
	r.Use(NewChiMiddleware("test-service", nil))
	r.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	spans := recorder.Ended()
	require.Len(t, spans, 1, "expected one span")

	span := spans[0]
	assert.Equal(t, trace.SpanKindServer, span.SpanKind())
	// Catch-all route pattern is "/*"
	assert.Equal(t, "/*", span.Name(), "span name should be catch-all route pattern")
}

// TestChiMiddleware_MultipleSkippedPaths verifies multiple paths can be skipped.
func TestChiMiddleware_MultipleSkippedPaths(t *testing.T) {
	recorder, tp := setupTracing(t)
	defer tp.Shutdown(context.Background())

	r := chi.NewRouter()
	r.Use(NewChiMiddleware("test-service", []string{"/health", "/ready", "/metrics"}))
	r.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Skipped paths
	skippedPaths := []string{"/health", "/ready", "/metrics"}
	for _, path := range skippedPaths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
	}

	spans := recorder.Ended()
	assert.Empty(t, spans, "all skipped paths should produce zero spans")

	// Reset for traced paths
	recorder, tp = setupTracing(t)
	defer tp.Shutdown(context.Background())

	r2 := chi.NewRouter()
	r2.Use(NewChiMiddleware("test-service", []string{"/health", "/ready", "/metrics"}))
	r2.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Traced paths
	tracedPaths := []string{"/api/users", "/api/orders"}
	for _, path := range tracedPaths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		r2.ServeHTTP(rec, req)
	}

	spans = recorder.Ended()
	assert.Len(t, spans, len(tracedPaths), "non-skipped paths should produce spans")
}

// TestChiMiddleware_HandlerStillExecutesWhenSkipped verifies the wrapped handler
// executes even when tracing is skipped.
func TestChiMiddleware_HandlerStillExecutesWhenSkipped(t *testing.T) {
	_, tp := setupTracing(t)
	defer tp.Shutdown(context.Background())

	sideEffect := false
	r := chi.NewRouter()
	r.Use(NewChiMiddleware("test-service", []string{"/health"}))
	r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
		sideEffect = true
		w.WriteHeader(http.StatusCreated)
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.True(t, sideEffect, "handler should execute even when tracing skipped")
	assert.Equal(t, http.StatusCreated, rec.Code, "handler response should be preserved")
}

// TestChiMiddleware_SpanAttributes verifies span contains expected HTTP attributes.
func TestChiMiddleware_SpanAttributes(t *testing.T) {
	recorder, tp := setupTracing(t)
	defer tp.Shutdown(context.Background())

	r := chi.NewRouter()
	r.Use(NewChiMiddleware("test-service", nil))
	r.Get("/api/test", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	spans := recorder.Ended()
	require.Len(t, spans, 1)

	span := spans[0]
	attrs := span.Attributes()

	// Verify HTTP method attribute
	methodAttr := getAttribute(attrs, "http.method")
	require.NotNil(t, methodAttr, "http.method attribute should be set")
	assert.Equal(t, http.MethodGet, methodAttr.Value.AsString())

	// Verify HTTP status code attribute
	statusAttr := getAttribute(attrs, "http.status_code")
	require.NotNil(t, statusAttr, "http.status_code attribute should be set")
	assert.Equal(t, int64(http.StatusOK), statusAttr.Value.AsInt64())
}

// getAttribute finds an attribute by key from a slice of attributes.
func getAttribute(attrs []attribute.KeyValue, key string) *attribute.KeyValue {
	for _, attr := range attrs {
		if string(attr.Key) == key {
			return &attr
		}
	}
	return nil
}

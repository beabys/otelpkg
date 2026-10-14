// tracing initialises the OpenTelemetry TracerProvider for the
// auth service. When tracing is disabled it returns a no-op shutdown so
// callers do not need to branch on the enabled flag.
package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.34.0"
)

// Init configures the global OpenTelemetry TracerProvider using cfg. When
// cfg.Enabled is false it returns a no-op shutdown func and nil error without
// dialling any endpoint. When enabled it builds an OTLP gRPC exporter,
// registers the provider globally, and returns a shutdown func that flushes
// and stops the exporter.
func Init(cfg Config) (shutdown func(context.Context) error, err error) {
	if !cfg.Enabled {
		return func(context.Context) error { return nil }, nil
	}

	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("tracing enabled but endpoint is empty")
	}
	if cfg.ServiceName == "" {
		return nil, fmt.Errorf("tracing enabled but service_name is empty")
	}

	ratio := cfg.SampleRatio
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}

	ctx := context.Background()

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("create otlp exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion("0.0.1"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	otel.SetTracerProvider(tp)

	// Set global propagator to extract/inject W3C trace context + baggage.
	// This must run BEFORE HTTP router and gRPC handlers are created so
	// otelchi/otelgrpc can extract inbound traceparent and inject outbound.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	shutdownFn := func(ctx context.Context) error {
		return tp.Shutdown(ctx)
	}
	return shutdownFn, nil
}

package tracing

// Config holds the parameters needed to initialise the OpenTelemetry tracing
// provider. It is intentionally decoupled from pkg/config so the tracing
// package can be tested and reused independently.
type Config struct {
	// Enabled controls whether an OTLP exporter is created. When false, Init
	// returns a no-op shutdown and does not dial any endpoint.
	Enabled bool
	// Endpoint is the OTLP gRPC collector address (host:port).
	Endpoint string
	// ServiceName is recorded as the `service.name` resource attribute.
	ServiceName string
	// SampleRatio is the trace ID ratio sampler value (0.0–1.0).
	SampleRatio float64
}

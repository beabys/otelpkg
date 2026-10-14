# otelpkg

OpenTelemetry tracing helpers for Go services. Provides framework-native
tracing middleware/handlers with path-based skip lists, plus a small
`tracing` package that wires an OTLP gRPC exporter and W3C context
propagation into the global OpenTelemetry provider.

- **Module:** `github.com/beabys/otelpkg` (Go 1.26)
- **Root package:** `otelpkg` — chi middleware + gRPC stats handler
- **Subpackage:** `tracing` — `TracerProvider` initialisation

## Install

```bash
go get github.com/beabys/otelpkg
```

## Usage

### chi middleware

`NewChiMiddleware` returns a native chi middleware
(`func(http.Handler) http.Handler`) that creates one server span per
request, skipping any path in `skipPaths`. The skip set is built once at
construction time, so per-request lookup is O(1).

```go
import (
    "github.com/go-chi/chi/v5"
    otel "github.com/beabys/otelpkg"
)

r := chi.NewRouter()
r.Use(otel.NewChiMiddleware("my-service", []string{"/health", "/ready"}))
r.Get("/users/{id}", handler)
```

> **Important:** mount the middleware on a `chi.NewRouter()`. `otelchi`
> v0.12.3 requires a chi route context and will panic if the middleware
> runs around a bare `http.Handler` that is not mounted on a chi router.

### gRPC stats handler

`NewGRPCServerHandler` returns a `stats.Handler` that creates one server
span per RPC, skipping any full method name in `skipMethods`.

```go
import (
    "google.golang.org/grpc"
    otel "github.com/beabys/otelpkg"
)

handler := otel.NewGRPCServerHandler([]string{
    "/grpc.health.v1.Health/Check",
    "/grpc.health.v1.Health/Watch",
})
srv := grpc.NewServer(grpc.StatsHandler(handler))
```

### TracerProvider initialisation

`tracing.Init` configures the global OpenTelemetry `TracerProvider` and
propagator. When `Enabled` is false it returns a no-op shutdown and dials
nothing, so callers never need to branch on the flag.

```go
import "github.com/beabys/otelpkg/tracing"

shutdown, err := tracing.Init(tracing.Config{
    Enabled:     true,
    Endpoint:    "localhost:4317",
    ServiceName: "my-service",
    SampleRatio: 1.0,
})
if err != nil {
    log.Fatal(err)
}
defer shutdown(context.Background())
```

`Init` validates that `Endpoint` and `ServiceName` are set when enabled,
clamps `SampleRatio` to `[0, 1]`, sets a parent-based trace-ID-ratio
sampler, and installs a W3C `TraceContext` + `Baggage` propagator. Call it
**before** creating routers and gRPC servers so inbound `traceparent` is
extracted and outbound context injected.

## Skip-list semantics

Skip lists are **exact match** (`==`), not prefix or glob. An empty or nil
list traces every request. For both transports the skip set is constructed
once and reused.

| Configured skip | `/health` | `/health/` | `/healthz` | `/api/health` | `/roles` |
|---|---|---|---|---|---|
| `/health` | skip | trace | trace | trace | trace |
| *(empty)* | trace | trace | trace | trace | trace |

## Testing

```bash
go test -race ./...            # unit tests
go test -race -count=2 ./...   # repeat for flakiness
go vet ./...
gofmt -l .
go test -race -tags integration ./... # integration (in-process fake OTLP collector)
```

Unit tests use `tracetest.SpanRecorder` with an in-memory
`TracerProvider`; no network is required. The global provider is saved and
restored around each test to avoid cross-test pollution.

## Known issues

- **`otelchi` v0.12.3 span name** is the chi route pattern only (e.g.
  `/users/{id}`), with no HTTP method prefix.
- **Plain `http.Handler` unsupported** — see the chi note above.

## License

MIT — see [LICENSE](LICENSE).

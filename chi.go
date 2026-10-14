package otelpkg

import (
	"net/http"

	"github.com/riandyrn/otelchi"
)

// NewChiMiddleware returns a chi-compatible tracing middleware
// (func(http.Handler) http.Handler): a server span per request, skipping
// paths in skipPaths. The path matcher is built once at construction time.
//
// Future framework-specific constructors (e.g. NewGinMiddleware) follow the
// same contract: return the framework's native middleware signature.
func NewChiMiddleware(serviceName string, skipPaths []string) func(http.Handler) http.Handler {
	pf := newPathFilter(skipPaths)
	return otelchi.Middleware(serviceName, otelchi.WithFilter(func(req *http.Request) bool {
		return pf.shouldTrace(req.URL.Path)
	}))
}

// otelpkg provides OpenTelemetry tracing utilities.
package otelpkg

// pathFilter decides whether a request path should be traced based on a
// configured skip list. It is transport-agnostic (no net/http dependency).
type pathFilter struct {
	skip map[string]struct{}
}

// newPathFilter builds a pathFilter from the given skip paths. The skip set
// is constructed once; subsequent shouldTrace calls are O(1).
func newPathFilter(skipPaths []string) pathFilter {
	skip := make(map[string]struct{}, len(skipPaths))
	for _, p := range skipPaths {
		skip[p] = struct{}{}
	}
	return pathFilter{skip: skip}
}

// shouldTrace returns false when path is in the skip set. An empty skip set
// traces all paths.
func (f pathFilter) shouldTrace(path string) bool {
	if len(f.skip) == 0 {
		return true
	}
	_, skip := f.skip[path]
	return !skip
}

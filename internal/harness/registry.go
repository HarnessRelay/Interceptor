package harness

import "sort"

// Registry selects the best adapter for a launch spec.
type Registry struct {
	adapters []Adapter
}

// NewRegistry creates a registry ordered by adapter priority.
func NewRegistry(adapters ...Adapter) *Registry {
	r := &Registry{}
	for _, adapter := range adapters {
		r.Register(adapter)
	}
	return r
}

// Register adds an adapter to the registry.
func (r *Registry) Register(adapter Adapter) {
	if adapter == nil {
		return
	}
	r.adapters = append(r.adapters, adapter)
	sort.SliceStable(r.adapters, func(i, j int) bool {
		return r.adapters[i].Priority() > r.adapters[j].Priority()
	})
}

// Adapters returns the registered adapters in selection order.
func (r *Registry) Adapters() []Adapter {
	out := make([]Adapter, len(r.adapters))
	copy(out, r.adapters)
	return out
}

// Select returns the highest-priority matching adapter.
func (r *Registry) Select(spec LaunchSpec) (Adapter, MatchResult, bool) {
	var best Adapter
	var bestMatch MatchResult
	for _, adapter := range r.adapters {
		match := adapter.Match(spec)
		if !match.Matched {
			continue
		}
		if best == nil || adapter.Priority() > best.Priority() || (adapter.Priority() == best.Priority() && match.Confidence > bestMatch.Confidence) {
			best = adapter
			bestMatch = match
		}
	}
	if best == nil {
		return nil, MatchResult{}, false
	}
	return best, bestMatch, true
}

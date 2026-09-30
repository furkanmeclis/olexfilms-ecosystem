package ioengine

import "fmt"

// Registry maps resource slugs to adapters.
type Registry struct {
	byResource map[string]ResourceAdapter
}

// NewRegistry builds an adapter registry.
func NewRegistry(adapters ...ResourceAdapter) *Registry {
	m := make(map[string]ResourceAdapter, len(adapters))
	for _, a := range adapters {
		if a == nil {
			continue
		}
		m[a.Resource()] = a
	}
	return &Registry{byResource: m}
}

// Get returns an adapter by resource slug.
func (r *Registry) Get(resource string) (ResourceAdapter, error) {
	a, ok := r.byResource[resource]
	if !ok {
		return nil, fmt.Errorf("ioengine: unknown resource %q", resource)
	}
	return a, nil
}

// Resources lists registered resource slugs.
func (r *Registry) Resources() []string {
	out := make([]string, 0, len(r.byResource))
	for k := range r.byResource {
		out = append(out, k)
	}
	return out
}

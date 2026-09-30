package searchengine

import "fmt"

// Registry maps spec ids to adapters.
type Registry struct {
	bySpec map[string]Adapter
}

// NewRegistry builds an adapter registry.
func NewRegistry(adapters ...Adapter) *Registry {
	m := make(map[string]Adapter, len(adapters))
	for _, a := range adapters {
		if a == nil {
			continue
		}
		spec := a.Spec()
		m[spec.ID] = a
	}
	return &Registry{bySpec: m}
}

// Get returns an adapter by spec id.
func (r *Registry) Get(spec string) (Adapter, error) {
	a, ok := r.bySpec[spec]
	if !ok {
		return nil, fmt.Errorf("searchengine: unknown spec %q", spec)
	}
	return a, nil
}

// Specs returns all registered spec metadata.
func (r *Registry) Specs() []Spec {
	out := make([]Spec, 0, len(r.bySpec))
	for _, a := range r.bySpec {
		out = append(out, a.Spec())
	}
	return out
}

// SpecIDs lists registered spec ids.
func (r *Registry) SpecIDs() []string {
	out := make([]string, 0, len(r.bySpec))
	for id := range r.bySpec {
		out = append(out, id)
	}
	return out
}

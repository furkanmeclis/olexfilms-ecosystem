package bulkengine

import "fmt"

// Registry maps resource slugs to bulk adapters.
type Registry struct {
	byResource map[string]BulkAdapter
}

// NewRegistry builds a bulk adapter registry.
func NewRegistry(adapters ...BulkAdapter) *Registry {
	m := make(map[string]BulkAdapter, len(adapters))
	for _, a := range adapters {
		if a == nil {
			continue
		}
		m[a.Resource()] = a
	}
	return &Registry{byResource: m}
}

// Get returns an adapter by resource slug.
func (r *Registry) Get(resource string) (BulkAdapter, error) {
	a, ok := r.byResource[resource]
	if !ok {
		return nil, fmt.Errorf("bulkengine: unknown resource %q", resource)
	}
	return a, nil
}

// ActionDef returns the action definition for a resource/action pair.
func (r *Registry) ActionDef(resource, action string) (BulkActionDef, BulkAdapter, error) {
	adapter, err := r.Get(resource)
	if err != nil {
		return BulkActionDef{}, nil, err
	}
	for _, def := range adapter.BulkActions() {
		if def.ID == action {
			return def, adapter, nil
		}
	}
	return BulkActionDef{}, nil, fmt.Errorf("bulkengine: unknown action %q for resource %q", action, resource)
}

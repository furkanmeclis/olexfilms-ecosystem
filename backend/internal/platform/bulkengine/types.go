package bulkengine

import "context"

// BulkActionParam is an extra input collected before a parameterized bulk action.
type BulkActionParam struct {
	Key      string `json:"key"`
	Kind     string `json:"kind"` // percent | number
	Required bool   `json:"required"`
	LabelKey string `json:"label_key"`
}

// BulkActionDef describes one bulk action exposed via resource meta.
type BulkActionDef struct {
	ID          string            `json:"id"`
	LabelKey    string            `json:"label_key"`
	Permission  string            `json:"permission"`
	Destructive bool              `json:"destructive"`
	Reversible  bool              `json:"reversible"`
	ConfirmKey  string            `json:"confirm_key,omitempty"`
	Params      []BulkActionParam `json:"params,omitempty"`
}

// BulkTarget selects rows by explicit ids or list query filters.
type BulkTarget struct {
	Scope  string            `json:"scope"` // ids | query
	IDs    []string          `json:"ids,omitempty"`
	Query  map[string]string `json:"query,omitempty"`
	Params map[string]string `json:"params,omitempty"`
}

type runKey struct{}

// Run is the stamped bulk execution context (params + query) available to adapters.
type Run struct {
	Params map[string]string
	Query  map[string]string
}

// WithRun stores bulk run data for ApplyItem (sync handlers and async workers).
func WithRun(ctx context.Context, run Run) context.Context {
	return context.WithValue(ctx, runKey{}, run)
}

// RunFrom returns bulk run data when present.
func RunFrom(ctx context.Context) (Run, bool) {
	run, ok := ctx.Value(runKey{}).(Run)
	return run, ok
}

// BulkItemResult is the outcome of applying one bulk item.
type BulkItemResult struct {
	EntityUUID string
	EntityType string
	OK         bool
	Error      string
	Op         string // update | delete
	Previous   map[string]any
}

// BulkSummary aggregates job results.
type BulkSummary struct {
	Total     int `json:"total"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
}

// BulkAdapter connects bulkengine to a list resource.
type BulkAdapter interface {
	Resource() string
	BulkActions() []BulkActionDef
	ResolveTargets(ctx context.Context, action string, target BulkTarget) ([]string, error)
	ApplyItem(ctx context.Context, action, entityUUID string) (BulkItemResult, error)
	RevertItem(ctx context.Context, action, entityUUID string, previous map[string]any) error
}

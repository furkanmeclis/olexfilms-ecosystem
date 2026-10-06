package bulkengine

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
)

// BulkActionParam is an extra input collected before a parameterized bulk action.
type BulkActionParam struct {
	Key      string `json:"key"`
	Kind     string `json:"kind"` // percent | number | uuid | enum | text
	Required bool   `json:"required"`
	LabelKey string `json:"label_key"`
	// Options are the allowed values of an enum param (TEC-371); the label
	// of a value is LabelKey + "." + value.
	Options []string `json:"options,omitempty"`
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
	// Applied is the state after the action (the keys the action set). Undo
	// compares it with the live row and skips a record changed since
	// (TEC-212); nil disables the check for this item.
	Applied map[string]any
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

// TxBinder is implemented by adapters that can run on a transaction: the
// bulk service applies every item and writes the bulk_operations log row
// on the same transaction (TEC-212).
type TxBinder interface {
	WithQueries(q *db.Queries) BulkAdapter
}

// StateReader is implemented by adapters whose undo must skip records
// changed after the bulk action: CurrentState returns the live values of
// the keys an ApplyItem result put in Applied. ErrEntityGone marks a
// record that no longer exists.
type StateReader interface {
	CurrentState(ctx context.Context, action, entityUUID string) (map[string]any, error)
}

// Bind returns the adapter bound to q when it supports transactions, else
// the adapter itself.
func Bind(a BulkAdapter, q *db.Queries) BulkAdapter {
	if b, ok := a.(TxBinder); ok && q != nil {
		return b.WithQueries(q)
	}
	return a
}

// ErrEntityGone is returned by StateReader.CurrentState for a record that
// no longer exists; undo reports it as skipped.
var ErrEntityGone = errors.New("bulkengine: entity gone")

// SameState reports whether live holds every key of applied with an equal
// JSON value (numbers compared as float64, slices element-wise).
func SameState(applied, live map[string]any) bool {
	for k, want := range applied {
		got, ok := live[k]
		if !ok || !jsonEqual(want, got) {
			return false
		}
	}
	return true
}

func jsonEqual(a, b any) bool {
	ab, errA := json.Marshal(a)
	bb, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	var av, bv any
	if json.Unmarshal(ab, &av) != nil || json.Unmarshal(bb, &bv) != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
}

package searchengine

import (
	"context"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// Spec ids of the TEC-209 record indexes. They live here so modules can
// refresh each other's documents (a customer change touches services,
// vehicles and warranties) without importing each other.
const (
	SpecServices   = "services"
	SpecWarranties = "warranties"
	SpecVehicles   = "vehicles"
)

// ListFinder runs a filtered index query for a module list endpoint
// (*Client). The list loads the hits from Postgres with its own scope
// filter afterwards (TEC-164 / TEC-209), so the index is never the only
// access check.
type ListFinder interface {
	Enabled() bool
	SearchIDs(ctx context.Context, spec, q, filter string, limit, offset int) ([]string, int64, error)
}

// Filter builds a Meilisearch filter expression from AND-ed parts.
type Filter struct{ parts []string }

// Eq adds `field = value` (skipped when value is 0).
func (f *Filter) Eq(field string, value int64) *Filter {
	if value != 0 {
		f.parts = append(f.parts, field+" = "+strconv.FormatInt(value, 10))
	}
	return f
}

// EqString adds `field = "value"` (skipped when value is empty).
func (f *Filter) EqString(field, value string) *Filter {
	if value = strings.TrimSpace(value); value != "" {
		f.parts = append(f.parts, field+" = "+strconv.Quote(value))
	}
	return f
}

// In adds `field IN [...]`. A nil slice means no restriction.
func (f *Filter) In(field string, values []int64) *Filter {
	if values == nil {
		return f
	}
	strs := make([]string, len(values))
	for i, v := range values {
		strs[i] = strconv.FormatInt(v, 10)
	}
	f.parts = append(f.parts, field+" IN ["+strings.Join(strs, ", ")+"]")
	return f
}

// String joins the parts with AND.
func (f *Filter) String() string { return strings.Join(f.parts, " AND ") }

// ParseUUIDs turns index hit ids into uuids (rank order, invalid and
// duplicate ids dropped) plus a rank map for reordering the rows Postgres
// returns.
func ParseUUIDs(ids []string) ([]uuid.UUID, map[uuid.UUID]int) {
	uuids := make([]uuid.UUID, 0, len(ids))
	rank := make(map[uuid.UUID]int, len(ids))
	for _, id := range ids {
		u, err := uuid.Parse(id)
		if err != nil {
			continue
		}
		if _, dup := rank[u]; dup {
			continue
		}
		rank[u] = len(uuids)
		uuids = append(uuids, u)
	}
	return uuids, rank
}

// Reorder returns rows in index rank order; rows the index did not name
// (or Postgres dropped as out of scope) are left out.
func Reorder[T any](rows []T, key func(T) uuid.UUID, rank map[uuid.UUID]int) []T {
	ordered := make([]T, len(rank))
	present := make([]bool, len(rank))
	for _, r := range rows {
		if i, ok := rank[key(r)]; ok && i < len(ordered) {
			ordered[i], present[i] = r, true
		}
	}
	out := make([]T, 0, len(rows))
	for i := range ordered {
		if present[i] {
			out = append(out, ordered[i])
		}
	}
	return out
}

// Keywords trims and drops empty values.
func Keywords(vals ...string) []string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

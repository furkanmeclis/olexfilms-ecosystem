package ioengine

import (
	"sort"
	"strings"
)

// TEC-211: central column visibility. A Column may name the permission
// slug that unlocks it (Column.Permission, e.g. pricing.purchase.read for a
// purchase price). The HTTP request that queues an export stores the
// subset of those slugs the actor holds on the job (query_json, reserved
// key QueryGrantedPermissions) and the worker re-evaluates the dataset
// against that stored set: columns whose permission is not in it are
// removed from the columns, the rows and the totals before encoding, so
// the file never carries a value the requester may not read, whatever
// format or adapter produced it.

// QueryGrantedPermissions is the reserved export query key holding the
// comma separated permission slugs granted to the job (written by
// RequestExport, never by clients).
const QueryGrantedPermissions = "_permissions"

// ColumnPermissions lists the distinct permission slugs the columns need,
// sorted.
func ColumnPermissions(cols []Column) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, c := range cols {
		slug := strings.TrimSpace(c.Permission)
		if slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		out = append(out, slug)
	}
	sort.Strings(out)
	return out
}

// SetGrantedPermissions stores the granted slugs on the query (an empty
// list removes the key).
func SetGrantedPermissions(q ExportQuery, slugs []string) {
	if q == nil {
		return
	}
	clean := make([]string, 0, len(slugs))
	for _, s := range slugs {
		if s = strings.TrimSpace(s); s != "" {
			clean = append(clean, s)
		}
	}
	if len(clean) == 0 {
		delete(q, QueryGrantedPermissions)
		return
	}
	sort.Strings(clean)
	q[QueryGrantedPermissions] = strings.Join(clean, ",")
}

// GrantedPermissions reads the granted slugs stored on the query.
func GrantedPermissions(q ExportQuery) map[string]bool {
	out := map[string]bool{}
	for _, s := range strings.Split(q[QueryGrantedPermissions], ",") {
		if s = strings.TrimSpace(s); s != "" {
			out[s] = true
		}
	}
	return out
}

// Granted reports whether the query holds the permission slug.
func Granted(q ExportQuery, slug string) bool {
	return GrantedPermissions(q)[slug]
}

// VisibleColumns keeps the columns without a permission and those whose
// permission is granted.
func VisibleColumns(cols []Column, granted map[string]bool) []Column {
	out := make([]Column, 0, len(cols))
	for _, c := range cols {
		if c.Permission == "" || granted[c.Permission] {
			out = append(out, c)
		}
	}
	return out
}

// ApplyColumnVisibility removes the columns the granted set does not
// unlock from the dataset: the column list, every row and the totals. The
// dataset is returned unchanged when no column needs a permission.
func ApplyColumnVisibility(ds Dataset, granted map[string]bool) Dataset {
	hidden := map[string]bool{}
	for _, c := range ds.Columns {
		if c.Permission != "" && !granted[c.Permission] {
			hidden[c.Key] = true
		}
	}
	if len(hidden) == 0 {
		return ds
	}
	ds.Columns = VisibleColumns(ds.Columns, granted)
	for _, row := range ds.Rows {
		for k := range hidden {
			delete(row, k)
		}
	}
	for k := range hidden {
		delete(ds.Totals, k)
	}
	return ds
}

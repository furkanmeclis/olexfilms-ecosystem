package apiquery

import (
	"fmt"
	"sort"
)

// SortSpec is the sort contract of one list endpoint: the whitelist of API
// field names (mapped to the trusted sort keys the SQL CASE matches on) and
// the default applied when the request has no sort. See docs/list-contract.md.
type SortSpec struct {
	Columns SortColumns
	Default SortField
}

// ResolvedSort is the single primary sort the SQL layer applies. Key is the
// trusted sort key from SortSpec.Columns (never raw user input); the query
// always adds a unique tiebreak (id) after it.
type ResolvedSort struct {
	Key  string
	Desc bool
}

// String renders the sort in API form ("-created_at" / "name").
func (f SortField) String() string {
	if f.Desc {
		return "-" + f.Field
	}
	return f.Field
}

// DefaultString is the default sort in API form, for resource meta
// default_sort.
func (s SortSpec) DefaultString() string {
	return s.Default.String()
}

// Fields returns the sortable API field names in stable (alphabetical)
// order, for resource meta sortable_fields.
func (s SortSpec) Fields() []string {
	out := make([]string, 0, len(s.Columns))
	for f := range s.Columns {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// ResolveSort validates every requested sort field against the endpoint
// whitelist (unknown field → *ValidationError, 400 VALIDATION_ERROR) and
// returns the first one as the primary sort. Extra fields are accepted for
// backward compatibility but ignored. Without a sort the spec default is
// used.
func ResolveSort(sorts []SortField, spec SortSpec) (ResolvedSort, error) {
	if len(spec.Columns) == 0 {
		panic("apiquery: SortSpec without columns")
	}
	if len(sorts) == 0 {
		key, ok := spec.Columns[spec.Default.Field]
		if !ok {
			panic(fmt.Sprintf("apiquery: default sort %q is not in the whitelist", spec.Default.Field))
		}
		return ResolvedSort{Key: key, Desc: spec.Default.Desc}, nil
	}
	if err := ValidateSort(sorts, spec.Columns); err != nil {
		return ResolvedSort{}, err
	}
	return ResolvedSort{Key: spec.Columns[sorts[0].Field], Desc: sorts[0].Desc}, nil
}

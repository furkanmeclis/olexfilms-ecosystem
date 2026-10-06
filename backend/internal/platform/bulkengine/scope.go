package bulkengine

// TEC-371: the caller's resolved permission scope stamped on a tenant bulk
// run. ExecuteTenantScoped (bulk handler) writes these keys after decoding
// the request body, so a client cannot supply them; the stored target keeps
// them for async jobs and undo. Adapters of resources whose reach is a
// permission scope (not just the active organization) read them back and
// re-authorize them against the run organization.

import (
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
)

// Target query keys written by the server.
const (
	// QueryScope is "brand" (brand wide or all) or the comma separated
	// organization ids the scope reaches.
	QueryScope = "_scope"
	// QueryActorUserID is the requesting user (internal id).
	QueryActorUserID = "_actor_user_id"
)

// ScopeBrand is the encoded scope of a brand wide (or all) grant.
const ScopeBrand = "brand"

// EncodeScope encodes a resolved scope; ok is false when it reaches no
// organization at all.
func EncodeScope(f scopefilter.Filter) (string, bool) {
	ids := f.OrgIDsArg()
	if ids == nil {
		return ScopeBrand, true
	}
	if len(ids) == 0 {
		return "", false
	}
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = strconv.FormatInt(id, 10)
	}
	return strings.Join(strs, ","), true
}

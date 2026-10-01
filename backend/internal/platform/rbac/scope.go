package rbac

import "strings"

// Scope limits which records a permission grant reaches. A permission slug is
// `<module>.<action>`; each role grant carries one scope out of the scopes the
// permission's catalog entry allows.
//
// Internal scopes nest, broadest first:
//
//	all      every record (cross-brand; platform operators only)
//	brand    every organization of the active brand (center roles)
//	subtree  the active organization and every organization below it
//	         (a distributor and its dealers)
//	managed  records of the active organization
//	assigned records assigned to the user
//	own      records the user created
//
// customer is the portal scope: records of the customer user themself. It is
// not comparable with the internal scopes.
type Scope string

const (
	ScopeAll      Scope = "all"
	ScopeBrand    Scope = "brand"
	ScopeSubtree  Scope = "subtree"
	ScopeManaged  Scope = "managed"
	ScopeAssigned Scope = "assigned"
	ScopeOwn      Scope = "own"
	ScopeCustomer Scope = "customer"
)

// AllScopes lists every scope, broadest internal scope first.
var AllScopes = []Scope{ScopeAll, ScopeBrand, ScopeSubtree, ScopeManaged, ScopeAssigned, ScopeOwn, ScopeCustomer}

// ParseScope validates a scope string.
func ParseScope(s string) (Scope, bool) {
	for _, sc := range AllScopes {
		if string(sc) == s {
			return sc, true
		}
	}
	return "", false
}

func (s Scope) rank() int {
	switch s {
	case ScopeAll:
		return 6
	case ScopeBrand:
		return 5
	case ScopeSubtree:
		return 4
	case ScopeManaged:
		return 3
	case ScopeAssigned:
		return 2
	case ScopeOwn:
		return 1
	default:
		return 0
	}
}

// Covers reports whether a grant with scope s satisfies a check that needs at
// least scope need. The customer scope only satisfies itself.
func (s Scope) Covers(need Scope) bool {
	if s == ScopeCustomer || need == ScopeCustomer {
		return s == need
	}
	return need.rank() > 0 && s.rank() >= need.rank()
}

// Broader returns the broader of two grants for the same permission, used to
// merge grants from several roles. An internal scope wins over customer.
func Broader(a, b Scope) Scope {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	if b.rank() > a.rank() {
		return b
	}
	return a
}

// ParseGrant splits a "slug" or "slug:scope" token. The scope is empty when
// the token carries none or an unknown one.
func ParseGrant(token string) (string, Scope) {
	slug, raw, found := strings.Cut(strings.TrimSpace(token), ":")
	slug = strings.TrimSpace(slug)
	if !found {
		return slug, ""
	}
	sc, ok := ParseScope(strings.TrimSpace(raw))
	if !ok {
		return slug, ""
	}
	return slug, sc
}

// GrantScope picks the scope stored for a grant to a non-super_admin role:
// want when the permission allows it, otherwise the broadest allowed scope.
// ok is false when the permission may not be granted to such a role at all.
func GrantScope(allowed []string, superAdminOnly bool, want Scope) (Scope, bool) {
	if superAdminOnly {
		return "", false
	}
	scopes := make([]Scope, 0, len(allowed))
	for _, a := range allowed {
		if s, ok := ParseScope(a); ok {
			if s == want {
				return s, true
			}
			scopes = append(scopes, s)
		}
	}
	if len(scopes) == 0 {
		return "", false
	}
	return Broadest(scopes), true
}

// Broadest returns the broadest scope of a list (catalog order is not assumed).
func Broadest(scopes []Scope) Scope {
	var out Scope
	for _, s := range scopes {
		out = Broader(out, s)
	}
	return out
}

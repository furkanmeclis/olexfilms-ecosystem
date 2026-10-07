package features

import (
	"time"

	"github.com/google/uuid"
)

// Flag scopes (module_flags.scope).
const (
	ScopeSystem         = "system"
	ScopeOrg            = "org"
	ScopeDealerStandard = "dealer_standard"
)

// Flag sources (module_flags.source).
const (
	SourceDefault     = "default"
	SourceAdmin       = "admin"
	SourceDistributor = "distributor"
	SourceService     = "service"
)

// Resolved sources: where an effective value comes from. On top of the flag
// sources above a state may come from the module level (core), a system
// switch, the distributor's own value (upstream) or its dealer standard.
const (
	FromCore     = "core"
	FromSystem   = "system"
	FromDefault  = "default"
	FromUpstream = "upstream"
	FromStandard = "standard"
)

// Organization types the resolver distinguishes.
const (
	OrgCenter      = "center"
	OrgDistributor = "distributor"
	OrgDealer      = "dealer"
	// OrgFleet is outside the tree; the resolver does not see it (TEC-472).
	OrgFleet = "fleet"
)

// ModuleRow is a modules table row.
type ModuleRow struct {
	Key            string
	Level          Level
	DefaultEnabled bool
	Paid           bool
}

// FlagRow is a module_flags row.
type FlagRow struct {
	Scope     string
	OrgID     int64
	Key       string
	Enabled   bool
	Source    string
	SetBy     *Actor
	UpdatedAt time.Time
}

// Actor names who set a flag.
type Actor struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// OrgNode is the organization a snapshot is resolved for.
type OrgNode struct {
	ID         int64
	Type       string
	ParentID   int64
	ParentType string
}

// State is the effective value of one module for one organization.
type State struct {
	Key   string `json:"key"`
	Level Level  `json:"level"`
	// Enabled is the value RequireFeature and the menus use.
	Enabled bool `json:"enabled"`
	// Visible is false when the level above has no access (closed system
	// wide, or a dealer's distributor does not have it); the Özellikler page
	// lists only visible modules ("liste üst seviyenin erişimi kadar").
	Visible        bool       `json:"visible"`
	Paid           bool       `json:"paid"`
	DefaultEnabled bool       `json:"default_enabled"`
	Source         string     `json:"source"`
	SetBy          *Actor     `json:"set_by,omitempty"`
	UpdatedAt      *time.Time `json:"updated_at,omitempty"`
	// UpstreamEnabled is the value one level up (the system for a
	// distributor, the distributor for its dealers). Writes that switch a
	// module on below a closed level are refused.
	UpstreamEnabled bool `json:"upstream_enabled"`
	// AdminOverride: the platform admin set this organization's value
	// directly; distributors cannot change it.
	AdminOverride bool `json:"admin_override"`
}

// Input is everything the resolver reads.
type Input struct {
	Modules []ModuleRow
	Flags   []FlagRow
	Org     OrgNode
}

type flagKey struct {
	scope string
	org   int64
	key   string
}

// Resolve computes the effective module states of in.Org, in catalog order.
//
// Rules (TEC-86, orchestrator decisions):
//  1. core modules are always on.
//  2. a system switch that is off closes the module everywhere, without
//     exception (not even the admin's per-organization value).
//  3. a distributor (or center) uses its own value, else the module default.
//  4. a dealer under a distributor:
//     - an admin value on the dealer wins, even when the distributor is off;
//     - otherwise the module is off while the distributor has it off;
//     - otherwise its own value, else the distributor's dealer standard,
//     else the module default.
//
// Inheritance is computed here on read; no row is ever copied.
func Resolve(in Input) []State {
	idx := make(map[flagKey]FlagRow, len(in.Flags))
	for _, f := range in.Flags {
		org := f.OrgID
		if f.Scope == ScopeSystem {
			org = 0
		}
		idx[flagKey{f.Scope, org, f.Key}] = f
	}
	get := func(scope string, org int64, key string) (FlagRow, bool) {
		f, ok := idx[flagKey{scope, org, key}]
		return f, ok
	}
	underDistributor := in.Org.Type == OrgDealer && in.Org.ParentType == OrgDistributor && in.Org.ParentID != 0

	out := make([]State, 0, len(in.Modules))
	for _, m := range in.Modules {
		st := State{Key: m.Key, Level: m.Level, Paid: m.Paid, DefaultEnabled: m.DefaultEnabled}
		if m.Level == LevelCore {
			st.Enabled, st.Visible, st.UpstreamEnabled, st.Source = true, true, true, FromCore
			out = append(out, st)
			continue
		}
		if sys, ok := get(ScopeSystem, 0, m.Key); ok && !sys.Enabled {
			st.Source = FromSystem
			apply(&st, sys)
			st.Enabled, st.Visible, st.UpstreamEnabled = false, false, false
			out = append(out, st)
			continue
		}
		own, hasOwn := get(ScopeOrg, in.Org.ID, m.Key)
		st.AdminOverride = hasOwn && own.Source == SourceAdmin

		if !underDistributor {
			st.UpstreamEnabled, st.Visible = true, true
			if hasOwn {
				st.Enabled, st.Source = own.Enabled, own.Source
				apply(&st, own)
			} else {
				st.Enabled, st.Source = m.DefaultEnabled, FromDefault
			}
			out = append(out, st)
			continue
		}

		parentOn := m.DefaultEnabled
		if pf, ok := get(ScopeOrg, in.Org.ParentID, m.Key); ok {
			parentOn = pf.Enabled
		}
		st.UpstreamEnabled = parentOn
		switch {
		case st.AdminOverride:
			st.Enabled, st.Source, st.Visible = own.Enabled, own.Source, true
			apply(&st, own)
		case !parentOn:
			st.Enabled, st.Source, st.Visible = false, FromUpstream, false
		case hasOwn:
			st.Enabled, st.Source, st.Visible = own.Enabled, own.Source, true
			apply(&st, own)
		default:
			st.Visible = true
			if std, ok := get(ScopeDealerStandard, in.Org.ParentID, m.Key); ok {
				st.Enabled, st.Source = std.Enabled, FromStandard
				apply(&st, std)
			} else {
				st.Enabled, st.Source = m.DefaultEnabled, FromDefault
			}
		}
		out = append(out, st)
	}
	return out
}

func apply(st *State, f FlagRow) {
	st.SetBy = f.SetBy
	if !f.UpdatedAt.IsZero() {
		t := f.UpdatedAt
		st.UpdatedAt = &t
	}
}

// Lookup returns the state of key in a snapshot.
func Lookup(states []State, key string) (State, bool) {
	for _, s := range states {
		if s.Key == key {
			return s, true
		}
	}
	return State{}, false
}

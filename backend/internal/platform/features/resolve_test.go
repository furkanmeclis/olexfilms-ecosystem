package features

import "testing"

func catalogRows() []ModuleRow {
	out := make([]ModuleRow, 0, len(Modules))
	for _, m := range Modules {
		out = append(out, ModuleRow(m))
	}
	return out
}

const (
	distID   = 10
	dealerID = 20
)

var dealerNode = OrgNode{ID: dealerID, Type: OrgDealer, ParentID: distID, ParentType: OrgDistributor}
var distNode = OrgNode{ID: distID, Type: OrgDistributor, ParentID: 1, ParentType: OrgCenter}

func state(t *testing.T, flags []FlagRow, org OrgNode, key string) State {
	t.Helper()
	st, ok := Lookup(Resolve(Input{Modules: catalogRows(), Flags: flags, Org: org}), key)
	if !ok {
		t.Fatalf("no state for %s", key)
	}
	return st
}

func TestCatalogConsistent(t *testing.T) {
	seen := map[string]bool{}
	for _, m := range Modules {
		if seen[m.Key] {
			t.Fatalf("duplicate module %s", m.Key)
		}
		seen[m.Key] = true
		switch m.Level {
		case LevelCore, LevelStandard:
			if !m.DefaultEnabled {
				t.Fatalf("%s %s must default on", m.Level, m.Key)
			}
		case LevelAddon:
			if m.DefaultEnabled {
				t.Fatalf("add-on %s must default off", m.Key)
			}
		default:
			t.Fatalf("bad level %s", m.Level)
		}
	}
	if !IsCore(ModuleOrganizations) || IsCore(ModuleLeads) || IsCore("nope") {
		t.Fatal("IsCore")
	}
}

func TestCoreAlwaysOn(t *testing.T) {
	flags := []FlagRow{
		{Scope: ScopeSystem, Key: ModuleServices, Enabled: false},
		{Scope: ScopeOrg, OrgID: dealerID, Key: ModuleServices, Enabled: false, Source: SourceAdmin},
	}
	st := state(t, flags, dealerNode, ModuleServices)
	if !st.Enabled || st.Source != FromCore {
		t.Fatalf("core = %+v", st)
	}
}

func TestDefaults(t *testing.T) {
	if st := state(t, nil, dealerNode, ModuleLeads); !st.Enabled || st.Source != FromDefault || !st.Visible {
		t.Fatalf("standard default = %+v", st)
	}
	st := state(t, nil, dealerNode, ModuleAIAssistant)
	if st.Enabled || st.Visible || st.Source != FromUpstream {
		t.Fatalf("add-on under a distributor without it = %+v", st)
	}
	if st := state(t, nil, distNode, ModuleAIAssistant); st.Enabled || !st.Visible {
		t.Fatalf("add-on default on distributor = %+v", st)
	}
}

// Acceptance: a module closed system wide is off everywhere, admin values
// included.
func TestSystemClosedHasNoException(t *testing.T) {
	flags := []FlagRow{
		{Scope: ScopeSystem, Key: ModuleLeads, Enabled: false},
		{Scope: ScopeOrg, OrgID: distID, Key: ModuleLeads, Enabled: true, Source: SourceAdmin},
		{Scope: ScopeOrg, OrgID: dealerID, Key: ModuleLeads, Enabled: true, Source: SourceAdmin},
	}
	for _, n := range []OrgNode{distNode, dealerNode, {ID: 1, Type: OrgCenter}} {
		st := state(t, flags, n, ModuleLeads)
		if st.Enabled || st.Visible || st.Source != FromSystem || st.UpstreamEnabled {
			t.Fatalf("%s: %+v", n.Type, st)
		}
	}
}

func TestDealerFollowsDistributor(t *testing.T) {
	flags := []FlagRow{{Scope: ScopeOrg, OrgID: distID, Key: ModuleLeads, Enabled: false, Source: SourceAdmin}}
	st := state(t, flags, dealerNode, ModuleLeads)
	if st.Enabled || st.Visible || st.Source != FromUpstream {
		t.Fatalf("distributor off -> dealer off: %+v", st)
	}
	// The distributor's own (stale) dealer value cannot reopen it.
	flags = append(flags, FlagRow{Scope: ScopeOrg, OrgID: dealerID, Key: ModuleLeads, Enabled: true, Source: SourceDistributor})
	if st := state(t, flags, dealerNode, ModuleLeads); st.Enabled {
		t.Fatalf("distributor value below a closed distributor: %+v", st)
	}
}

// Admin exception: the admin opens a module for a dealer even though the
// distributor does not have it.
func TestAdminOverrideBelowClosedDistributor(t *testing.T) {
	flags := []FlagRow{
		{Scope: ScopeOrg, OrgID: dealerID, Key: ModuleAIAssistant, Enabled: true, Source: SourceAdmin},
	}
	st := state(t, flags, dealerNode, ModuleAIAssistant)
	if !st.Enabled || !st.Visible || !st.AdminOverride || st.Source != SourceAdmin || st.UpstreamEnabled {
		t.Fatalf("admin override = %+v", st)
	}
}

// New dealers get the dealer standard; the dealer's own value wins over it.
func TestDealerStandardInheritance(t *testing.T) {
	flags := []FlagRow{
		{Scope: ScopeOrg, OrgID: distID, Key: ModuleAIAssistant, Enabled: true, Source: SourceAdmin},
		{Scope: ScopeDealerStandard, OrgID: distID, Key: ModuleAIAssistant, Enabled: true, Source: SourceDistributor},
		{Scope: ScopeDealerStandard, OrgID: distID, Key: ModuleLeads, Enabled: false, Source: SourceDistributor},
	}
	if st := state(t, flags, dealerNode, ModuleAIAssistant); !st.Enabled || st.Source != FromStandard {
		t.Fatalf("standard add-on = %+v", st)
	}
	if st := state(t, flags, dealerNode, ModuleLeads); st.Enabled || !st.Visible || st.Source != FromStandard {
		t.Fatalf("standard off = %+v", st)
	}
	flags = append(flags, FlagRow{Scope: ScopeOrg, OrgID: dealerID, Key: ModuleLeads, Enabled: true, Source: SourceDistributor})
	if st := state(t, flags, dealerNode, ModuleLeads); !st.Enabled || st.Source != SourceDistributor {
		t.Fatalf("own value over standard = %+v", st)
	}
	// The standard does not apply to the distributor itself.
	if st := state(t, flags, distNode, ModuleLeads); !st.Enabled {
		t.Fatalf("distributor own = %+v", st)
	}
}

// A dealer directly under the center is not bound by a distributor.
func TestDealerUnderCenter(t *testing.T) {
	n := OrgNode{ID: dealerID, Type: OrgDealer, ParentID: 1, ParentType: OrgCenter}
	if st := state(t, nil, n, ModuleLeads); !st.Enabled || !st.Visible {
		t.Fatalf("dealer under center = %+v", st)
	}
	if st := state(t, nil, n, ModuleAIAssistant); st.Enabled || !st.Visible {
		t.Fatalf("add-on under center = %+v", st)
	}
}

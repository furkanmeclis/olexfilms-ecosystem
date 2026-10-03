// Package migrator imports the legacy Olex hub and warehouse databases into
// this application (design §7, K26/K27, TEC-252).
//
// A run executes the steps of a profile in order. Each step reads the legacy
// sources through the read-only source.LegacySource, writes to the new
// Postgres in its own transaction and records legacy id -> new uuid pairs in
// migration_map through the Mapper, which makes reruns idempotent. Every run
// and step is logged in migration_runs.
package migrator

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Source system keys (migration_map.source_system and source names).
const (
	SourceHub = "hub"
	SourceWH  = "wh"
)

// Profile is a named set of legacy sources and the steps that import them.
type Profile struct {
	Name string
	// Enabled false refuses to run (Glorian stays in its own install, K2).
	Enabled bool
	// Sources are opened before the first step; a run without steps opens
	// none.
	Sources []string
	// Steps returns the ordered steps of the profile.
	Steps func() []Step
}

// ErrUnknownProfile and ErrProfileDisabled are returned by Lookup.
var (
	ErrUnknownProfile  = errors.New("migrator: unknown profile")
	ErrProfileDisabled = errors.New("migrator: profile is disabled")
	ErrUnknownStep     = errors.New("migrator: unknown step")
)

// olexSteps lists the Olex import steps in run order. F2-01c onwards add
// them here (organizations, users, customers, catalog, stock, services, ...).
func olexSteps() []Step {
	return []Step{
		OrganizationsStep{}, // TEC-254: center, TR distributor, dealers
		UsersStep{},         // TEC-254: users, memberships, roles
		CustomersStep{},     // TEC-255: customers -> users, profiles, dealer links

		// TEC-256: car brands, models and brand logos; product categories,
		// products and the warehouse product match.
		VehicleCatalogStep{},
		CatalogStep{},

		// TEC-257: warehouse structure, stock units and their initial
		// ownership.
		WarehousesStep{},
		UnitsStep{},

		// TEC-258: legacy stock movements -> ledger opening, projection
		// rebuild.
		LedgerStep{},

		// TEC-261: hub + warehouse orders, merged by external_reference.
		OrdersStep{},
	}
}

// Profiles returns the built-in profiles.
func Profiles() map[string]Profile {
	return map[string]Profile{
		"olex": {
			Name:    "olex",
			Enabled: true,
			Sources: []string{SourceHub, SourceWH},
			Steps:   olexSteps,
		},
		// Glorian is not migrated in this scope (K2): the profile is kept so
		// a later import is configuration, but it refuses to run.
		"glorian": {
			Name:    "glorian",
			Enabled: false,
			Sources: []string{SourceHub, SourceWH},
			Steps:   func() []Step { return nil },
		},
	}
}

// Lookup returns the named profile, refusing unknown and disabled ones.
func Lookup(profiles map[string]Profile, name string) (Profile, error) {
	p, ok := profiles[name]
	if !ok {
		names := make([]string, 0, len(profiles))
		for n := range profiles {
			names = append(names, n)
		}
		sort.Strings(names)
		return Profile{}, fmt.Errorf("%w %q (known: %s)", ErrUnknownProfile, name, strings.Join(names, ", "))
	}
	if !p.Enabled {
		return Profile{}, fmt.Errorf("%w: %q is registered but not enabled (design K2)", ErrProfileDisabled, name)
	}
	return p, nil
}

// SelectSteps returns the profile steps named in only, in profile order; an
// empty only selects every step. An unknown name is an error.
func SelectSteps(p Profile, only []string) ([]Step, error) {
	var all []Step
	if p.Steps != nil {
		all = p.Steps()
	}
	if len(only) == 0 {
		return all, nil
	}
	want := map[string]bool{}
	for _, n := range only {
		if n = strings.TrimSpace(n); n != "" {
			want[n] = true
		}
	}
	var out []Step
	for _, s := range all {
		if want[s.Name()] {
			out = append(out, s)
			delete(want, s.Name())
		}
	}
	if len(want) > 0 {
		missing := make([]string, 0, len(want))
		for n := range want {
			missing = append(missing, n)
		}
		sort.Strings(missing)
		return nil, fmt.Errorf("%w in profile %q: %s", ErrUnknownStep, p.Name, strings.Join(missing, ", "))
	}
	return out, nil
}

package migrator

import (
	"context"
	"fmt"
)

// UnitCreditsStep opens the usage credits of the migrated units (TEC-1221,
// kredili kod).
//
// The legacy hub has no credit or usage counter (stock_items, products and
// service_items carry none), so the credits are derived from the service
// history: a serial piece unit used in legacy services opens with
// credits_total = the product's usage_credits (5 by default, raised to the
// use count when a unit was used more often) and credits_remaining =
// total - non-cancelled services, except that a unit the hub consumed
// (status used) has no credit left. Units never used get no balance: they
// open with the default on their first use. It runs after unit_trace so the
// unit statuses are final; units that already have a balance are left
// alone, so a rerun writes nothing (unit_credits_backfill, migration
// 000180).
type UnitCreditsStep struct{}

// Name implements Step.
func (UnitCreditsStep) Name() string { return "unit_credits" }

// Run implements Step.
func (UnitCreditsStep) Run(ctx context.Context, _ Sources, dst *Target, _ *Mapper) (StepResult, error) {
	c := counts{}
	n, err := dst.Q.UnitCreditsBackfill(ctx)
	if err != nil {
		return StepResult{Counts: c}, fmt.Errorf("unit credits: %w", err)
	}
	c.add("credit_balances_opened", int64(n))
	return StepResult{Counts: c}, nil
}

package usecase

import (
	"context"
	"fmt"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/jackc/pgx/v5/pgtype"
)

// ChangeEvent is the leads.created / leads.updated outbox event of row
// (TEC-645). The payload is only a pointer: the search index sync rebuilds
// the document from Postgres.
func ChangeEvent(name string, row db.Lead, actor pgtype.Int8) events.Event {
	id, u := row.ID, row.Uuid
	ev := events.New(name).WithTenant(row.OrganizationID).WithEntity("lead", &id, &u).
		WithPayload(map[string]any{
			"lead_uuid":       row.Uuid.String(),
			"organization_id": row.OrganizationID,
			"brand_id":        row.BrandID,
		})
	if actor.Valid && actor.Int64 != 0 {
		ev = ev.WithActor(actor.Int64)
	}
	return ev
}

// EnqueueChange writes the change event of row through q, which must be
// bound to the transaction of the lead write. Every lead write path calls
// it so the search index never misses a lead.
func EnqueueChange(ctx context.Context, q *db.Queries, name string, row db.Lead, actor pgtype.Int8) error {
	if err := outbox.EnqueueQueries(ctx, q, ChangeEvent(name, row, actor)); err != nil {
		return fmt.Errorf("leads: outbox: %w", err)
	}
	return nil
}

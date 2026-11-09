package usecase

// TEC-1060 (RV-5.4.1): who did what on a stock entry and when, from its
// audit events (warehouse.stock_entry.*: created, header_updated,
// confirmed, cancelled).

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
)

// EntryUser names the user of an entry step.
type EntryUser struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// EntryStep is one recorded step of an entry. Action is the audit action
// without the "warehouse.stock_entry." prefix; User is null for a system
// step.
type EntryStep struct {
	Action string     `json:"action"`
	At     time.Time  `json:"at"`
	User   *EntryUser `json:"user"`
}

const entryAuditPrefix = AuditResourceStockEntry + "."

func entryTimeline(ctx context.Context, q *db.Queries, e db.StockEntry) ([]EntryStep, error) {
	rows, err := q.ListResourceActivityEvents(ctx, db.ListResourceActivityEventsParams{
		Resource: AuditResourceStockEntry, ResourceUuid: e.Uuid,
	})
	if err != nil {
		return nil, fmt.Errorf("warehouse: entry timeline: %w", err)
	}
	out := make([]EntryStep, 0, len(rows))
	for _, r := range rows {
		st := EntryStep{Action: strings.TrimPrefix(r.Action, entryAuditPrefix), At: r.CreatedAt.Time}
		if r.ActorUuid.Valid {
			st.User = &EntryUser{
				UUID: uuid.UUID(r.ActorUuid.Bytes),
				Name: strings.TrimSpace(r.ActorName.String + " " + r.ActorSurname.String),
			}
		}
		out = append(out, st)
	}
	return out, nil
}

package usecase

// TEC-1060 (RV-5.4.1): who did what on a stock entry and when, from its
// audit events (warehouse.stock_entry.*: created, header_updated,
// confirmed, cancelled; TEC-1225: lines_added, line_deleted, line_updated,
// placed, scanned, unscanned, labels_printed, discrepancies_recorded,
// attachment_added, attachment_removed, undone). Consecutive scans of one
// user are one step with a count.

import (
	"context"
	"encoding/json"
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
	// Details: the whitelisted payload of the step (barcode, count,
	// location, result, kind, filename, reason); never internal ids.
	Details map[string]any `json:"details,omitempty"`
}

// timelineDetailKeys are the payload keys a timeline step shows.
var timelineDetailKeys = []string{"barcode", "count", "location", "result", "kind", "filename", "reason", "lines"}

func stepDetails(payload []byte) map[string]any {
	if len(payload) == 0 {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(payload, &m) != nil {
		return nil
	}
	var out map[string]any
	for _, k := range timelineDetailKeys {
		v, ok := m[k]
		if !ok || v == nil || v == "" {
			continue
		}
		if out == nil {
			out = map[string]any{}
		}
		out[k] = v
	}
	return out
}

// collapsible actions merge into the previous step of the same user.
func collapsible(action string) bool {
	return action == "scanned" || action == "unscanned"
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
		st := EntryStep{Action: strings.TrimPrefix(r.Action, entryAuditPrefix), At: r.CreatedAt.Time, Details: stepDetails(r.Payload)}
		if r.ActorUuid.Valid {
			st.User = &EntryUser{
				UUID: uuid.UUID(r.ActorUuid.Bytes),
				Name: strings.TrimSpace(r.ActorName.String + " " + r.ActorSurname.String),
			}
		}
		if n := len(out); n > 0 && collapsible(st.Action) && out[n-1].Action == st.Action && sameUser(out[n-1].User, st.User) {
			prev := &out[n-1]
			cnt, _ := prev.Details["count"].(float64)
			if cnt == 0 {
				cnt = 1
			}
			if prev.Details == nil {
				prev.Details = map[string]any{}
			}
			prev.Details["count"] = cnt + 1
			prev.Details["barcode"] = st.Details["barcode"]
			delete(prev.Details, "result")
			prev.At = st.At
			continue
		}
		out = append(out, st)
	}
	return out, nil
}

func sameUser(a, b *EntryUser) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.UUID == b.UUID
}

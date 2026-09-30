package activity

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/netip"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Recorder persists audit events.
type Recorder struct {
	q   *db.Queries
	log *slog.Logger
}

// NewRecorder creates an activity recorder.
func NewRecorder(q *db.Queries, log *slog.Logger) *Recorder {
	if log == nil {
		log = slog.Default()
	}
	return &Recorder{q: q, log: log}
}

// Record writes an activity event (fail-soft).
func (r *Recorder) Record(ctx context.Context, actorID *int64, action, resource string, resourceUUID *uuid.UUID, payload map[string]any, rreq *http.Request) {
	if r == nil || r.q == nil {
		return
	}
	payload = withOriginPayload(ctx, payload)
	body, err := json.Marshal(payload)
	if err != nil || payload == nil {
		body = []byte("{}")
	}
	var ru pgtype.UUID
	if resourceUUID != nil {
		ru = pgtype.UUID{Bytes: *resourceUUID, Valid: true}
	}
	var ip *netip.Addr
	var ua pgtype.Text
	if rreq != nil {
		if host, _, err := net.SplitHostPort(rreq.RemoteAddr); err == nil {
			if parsed, err := netip.ParseAddr(host); err == nil {
				ip = &parsed
			}
		}
		if rreq.UserAgent() != "" {
			ua = pgtype.Text{String: rreq.UserAgent(), Valid: true}
		}
	}
	_, err = r.q.InsertActivityEvent(ctx, db.InsertActivityEventParams{
		ActorUserID:  pgtypeInt8(actorID),
		Action:       action,
		Resource:     resource,
		ResourceUuid: ru,
		Payload:      body,
		IpAddress:    ip,
		UserAgent:    ua,
	})
	if err != nil {
		r.log.Warn("activity_record_failed", "action", action, "error", err)
	}
}

func pgtypeInt8(id *int64) pgtype.Int8 {
	if id == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *id, Valid: true}
}

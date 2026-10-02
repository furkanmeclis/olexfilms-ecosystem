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
	if err := Write(ctx, r.q, actorID, action, resource, resourceUUID, payload, MetaFromRequest(rreq)); err != nil {
		r.log.Warn("activity_record_failed", "action", action, "error", err)
	}
}

// Meta is the request origin of an event (client IP and user agent).
type Meta struct {
	IP        *netip.Addr
	UserAgent string
}

// MetaFromRequest extracts the client IP and user agent (nil request: none).
func MetaFromRequest(rreq *http.Request) Meta {
	var m Meta
	if rreq == nil {
		return m
	}
	if host, _, err := net.SplitHostPort(rreq.RemoteAddr); err == nil {
		if parsed, err := netip.ParseAddr(host); err == nil {
			m.IP = &parsed
		}
	}
	m.UserAgent = rreq.UserAgent()
	return m
}

// Write inserts an activity event with q and returns the error. Use it with
// transaction queries when the audit row must commit (or roll back) together
// with the change it describes (e.g. KVKK anonymization, TEC-161).
func Write(ctx context.Context, q *db.Queries, actorID *int64, action, resource string, resourceUUID *uuid.UUID, payload map[string]any, meta Meta) error {
	payload = withOriginPayload(ctx, payload)
	body, err := json.Marshal(payload)
	if err != nil || payload == nil {
		body = []byte("{}")
	}
	var ru pgtype.UUID
	if resourceUUID != nil {
		ru = pgtype.UUID{Bytes: *resourceUUID, Valid: true}
	}
	ua := pgtype.Text{String: meta.UserAgent, Valid: meta.UserAgent != ""}
	_, err = q.InsertActivityEvent(ctx, db.InsertActivityEventParams{
		ActorUserID:  pgtypeInt8(actorID),
		Action:       action,
		Resource:     resource,
		ResourceUuid: ru,
		Payload:      body,
		IpAddress:    meta.IP,
		UserAgent:    ua,
	})
	return err
}

func pgtypeInt8(id *int64) pgtype.Int8 {
	if id == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *id, Valid: true}
}

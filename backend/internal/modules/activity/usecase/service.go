package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

var ErrNotFound = errors.New("not found")

// Service lists audit events.
type Service struct {
	q *db.Queries
}

// New creates an activity service.
func New(q *db.Queries) *Service {
	return &Service{q: q}
}

// Event is API projection.
type Event struct {
	UUID         uuid.UUID      `json:"uuid"`
	ActorUserID  *int64         `json:"actor_user_id,omitempty"`
	Action       string         `json:"action"`
	Resource     string         `json:"resource"`
	ResourceUUID *uuid.UUID     `json:"resource_uuid,omitempty"`
	Payload      map[string]any `json:"payload"`
	CreatedAt    time.Time      `json:"created_at"`
}

// List returns paginated activity events.
func (s *Service) List(
	ctx context.Context,
	q apiquery.Query,
	actorID *int64,
	resource, action, search string,
) ([]Event, int64, error) {
	params := db.ListActivityEventsParams{
		ActorUserID: pgtypeInt8(actorID),
		Resource:    textNarg(emptyToNil(resource)),
		Action:      textNarg(emptyToNil(action)),
		Q:           textNarg(emptyToNil(search)),
		LimitCount:  q.Limit,
		OffsetCount: q.Offset,
	}
	rows, err := s.q.ListActivityEvents(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountActivityEvents(ctx, db.CountActivityEventsParams{
		ActorUserID: params.ActorUserID,
		Resource:    params.Resource,
		Action:      params.Action,
		Q:           params.Q,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]Event, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapEvent(row))
	}
	return out, total, nil
}

func mapEvent(row db.ActivityEvent) Event {
	var payload map[string]any
	_ = json.Unmarshal(row.Payload, &payload)
	var actor *int64
	if row.ActorUserID.Valid {
		v := row.ActorUserID.Int64
		actor = &v
	}
	var ru *uuid.UUID
	if row.ResourceUuid.Valid {
		id := uuid.UUID(row.ResourceUuid.Bytes)
		ru = &id
	}
	return Event{
		UUID: row.Uuid, ActorUserID: actor, Action: row.Action, Resource: row.Resource,
		ResourceUUID: ru, Payload: payload, CreatedAt: row.CreatedAt.Time,
	}
}

func pgtypeInt8(id *int64) pgtype.Int8 {
	if id == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *id, Valid: true}
}

func textNarg(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func emptyToNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

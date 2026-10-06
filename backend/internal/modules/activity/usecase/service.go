package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
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

// List returns paginated activity events. params carries the filters and
// sort parsed by activity.ListParams (TEC-365).
func (s *Service) List(ctx context.Context, params db.ListActivityEventsParams, limit, offset int32) ([]Event, int64, error) {
	params.LimitCount, params.OffsetCount = limit, offset
	rows, err := s.q.ListActivityEvents(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountActivityEvents(ctx, db.CountActivityEventsParams{
		ActorUuid: params.ActorUuid, Resources: params.Resources, Actions: params.Actions,
		CreatedFrom: params.CreatedFrom, CreatedBefore: params.CreatedBefore, Q: params.Q,
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

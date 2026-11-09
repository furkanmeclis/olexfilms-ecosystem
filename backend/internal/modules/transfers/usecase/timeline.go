package usecase

// TEC-1060 (RV-5.4.1): who did what on a transfer and when, from the step
// columns of the request (requested_by / decided_by / shipped_by /
// received_by / cancelled_by with their times).

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// UserRef names a user on a transfer step.
type UserRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// TimelineStep is one step of a transfer: requested, approved, rejected,
// shipped, received or cancelled. User is null for a system step.
type TimelineStep struct {
	Action string    `json:"action"`
	At     time.Time `json:"at"`
	User   *UserRef  `json:"user"`
}

type stepCol struct {
	action string
	at     pgtype.Timestamptz
	user   pgtype.Int8
}

// timeline lists the recorded steps of a request, oldest first.
func timeline(ctx context.Context, q *db.Queries, r db.StockTransferRequest) ([]TimelineStep, error) {
	decision := StatusApproved
	if r.Status == StatusRejected {
		decision = StatusRejected
	}
	cols := []stepCol{
		{StatusRequested, r.CreatedAt, r.RequestedByUserID},
		{decision, r.DecidedAt, r.DecidedByUserID},
		{StatusShipped, r.ShippedAt, r.ShippedByUserID},
		{StatusReceived, r.ReceivedAt, r.ReceivedByUserID},
		{StatusCancelled, r.CancelledAt, r.CancelledByUserID},
	}
	var ids []int64
	for _, c := range cols {
		if c.at.Valid && c.user.Valid {
			ids = append(ids, c.user.Int64)
		}
	}
	users := map[int64]*UserRef{}
	if len(ids) > 0 {
		rows, err := q.ListUsersByIDs(ctx, ids)
		if err != nil {
			return nil, fmt.Errorf("transfers: timeline users: %w", err)
		}
		for _, u := range rows {
			users[u.ID] = &UserRef{UUID: u.Uuid, Name: strings.TrimSpace(u.Name + " " + u.Surname)}
		}
	}
	out := make([]TimelineStep, 0, len(cols))
	for _, c := range cols {
		if !c.at.Valid {
			continue
		}
		st := TimelineStep{Action: c.action, At: c.at.Time}
		if c.user.Valid {
			st.User = users[c.user.Int64]
		}
		out = append(out, st)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, nil
}

package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
)

// CreatorRef is the user who opened an order (TEC-1090, TEC-1055): the
// list and the detail read the same orders.created_by_user_id.
type CreatorRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// withCreators fills CreatedBy of views (same order as orders) with one
// user lookup per page. Orders without a creator (system or migrated
// rows) keep null.
func withCreators(ctx context.Context, q *db.Queries, orders []db.Order, views []OrderView) error {
	ids := make([]int64, 0, len(orders))
	seen := map[int64]bool{}
	for _, o := range orders {
		if o.CreatedByUserID.Valid && !seen[o.CreatedByUserID.Int64] {
			seen[o.CreatedByUserID.Int64] = true
			ids = append(ids, o.CreatedByUserID.Int64)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	users, err := q.ListUsersByIDs(ctx, ids)
	if err != nil {
		return fmt.Errorf("orders: creators: %w", err)
	}
	byID := make(map[int64]CreatorRef, len(users))
	for _, u := range users {
		name := strings.TrimSpace(strings.TrimSpace(u.Name) + " " + strings.TrimSpace(u.Surname))
		byID[u.ID] = CreatorRef{UUID: u.Uuid, Name: name}
	}
	for i, o := range orders {
		if !o.CreatedByUserID.Valid {
			continue
		}
		if ref, ok := byID[o.CreatedByUserID.Int64]; ok {
			views[i].CreatedBy = &ref
		}
	}
	return nil
}

package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// CreatorRef is the user who opened an order (TEC-1090, TEC-1055): the
// list and the detail read the same orders.created_by_user_id.
type CreatorRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// withCreators fills CreatedBy and Picker (TEC-1228) of views (same order
// as orders) with one user lookup per page. Orders without a creator
// (system or migrated rows) keep null.
func withCreators(ctx context.Context, q *db.Queries, orders []db.Order, views []OrderView) error {
	ids := make([]int64, 0, len(orders))
	seen := map[int64]bool{}
	add := func(id pgtype.Int8) {
		if id.Valid && !seen[id.Int64] {
			seen[id.Int64] = true
			ids = append(ids, id.Int64)
		}
	}
	for _, o := range orders {
		add(o.CreatedByUserID)
		add(o.PickerUserID)
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
		byID[u.ID] = CreatorRef{UUID: u.Uuid, Name: userName(u.Name, u.Surname)}
	}
	for i, o := range orders {
		if o.CreatedByUserID.Valid {
			if ref, ok := byID[o.CreatedByUserID.Int64]; ok {
				views[i].CreatedBy = &ref
			}
		}
		if o.PickerUserID.Valid {
			if ref, ok := byID[o.PickerUserID.Int64]; ok {
				views[i].Picker = &UserRef{UUID: ref.UUID, Name: ref.Name}
			}
		}
	}
	return nil
}

func userName(name, surname string) string {
	return strings.TrimSpace(strings.TrimSpace(name) + " " + strings.TrimSpace(surname))
}

package repository

import (
	"context"
	"errors"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// FindUserByPhone loads an active (not deleted) user by E.164 phone.
func (r *Postgres) FindUserByPhone(ctx context.Context, e164 string) (model.User, error) {
	row, err := r.q.GetUserByPhone(ctx, optText(e164))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.User{}, ErrNotFound
		}
		return model.User{}, err
	}
	return mapUser(row), nil
}

// CreatePhoneUser creates a phone-only account (no e-mail, unusable password
// hash) with a verified phone and the given system role (K11: customer).
func (r *Postgres) CreatePhoneUser(ctx context.Context, e164, passwordHash, locale, roleSlug string) (model.User, error) {
	var user model.User
	err := r.withTx(ctx, func(q *db.Queries) error {
		now := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
		row, err := q.CreateUser(ctx, db.CreateUserParams{
			PhoneE164: optText(e164), PhoneVerifiedAt: now, PasswordHash: passwordHash,
			Name: "", Surname: "", Status: "active",
		})
		if err != nil {
			return err
		}
		if locale != "" {
			// Empty keeps NULL: the locale is inherited (i18n.Resolve).
			if err := q.UpdateUserLocale(ctx, db.UpdateUserLocaleParams{ID: row.ID, Locale: optText(locale)}); err != nil {
				return err
			}
			row.Locale = optText(locale)
		}
		user = mapUser(row)
		if roleSlug == "" {
			return nil
		}
		return q.AssignUserRoleBySlug(ctx, db.AssignUserRoleBySlugParams{UserID: row.ID, Slug: roleSlug})
	})
	if err != nil {
		return model.User{}, err
	}
	return user, nil
}

// MarkPhoneVerified stamps phone_verified_at once.
func (r *Postgres) MarkPhoneVerified(ctx context.Context, userID int64) error {
	return r.q.MarkUserPhoneVerified(ctx, userID)
}

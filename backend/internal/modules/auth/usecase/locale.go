package usecase

import (
	"context"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/google/uuid"
)

// ResolveLocale returns the effective locale and timezone of a user in an
// organization context (K10): user -> active org -> brand center ->
// Accept-Language -> tr. Without an active organization the center of the
// request brand is used.
func (u *AuthUseCase) ResolveLocale(ctx context.Context, userID int64, orgUUID *uuid.UUID) (i18n.Resolved, error) {
	var brandID *int64
	if b, ok := brandctx.From(ctx); ok {
		id := b.ID
		brandID = &id
	}
	src, err := u.repo.LocaleSources(ctx, userID, orgUUID, brandID)
	if err != nil {
		return i18n.Resolved{}, err
	}
	src.AcceptLanguage = i18n.AcceptLanguage(ctx)
	return i18n.Resolve(src), nil
}

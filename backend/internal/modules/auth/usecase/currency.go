package usecase

import (
	"context"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/google/uuid"
)

// CurrencySource resolves the currency block of /auth/me (TEC-1214).
type CurrencySource func(ctx context.Context, orgUUID *uuid.UUID, brandID *int64) (brand, organization, display string, err error)

// SetCurrencySource attaches the /auth/me currency resolver (nil: the
// currency fields are left empty).
func (u *AuthUseCase) SetCurrencySource(src CurrencySource) {
	u.currencies = src
}

func (u *AuthUseCase) sessionCurrencies(ctx context.Context, orgUUID *uuid.UUID) (brand, organization, display string, err error) {
	if u.currencies == nil {
		return "", "", "", nil
	}
	var brandID *int64
	if b, ok := brandctx.From(ctx); ok {
		id := b.ID
		brandID = &id
	}
	return u.currencies(ctx, orgUUID, brandID)
}

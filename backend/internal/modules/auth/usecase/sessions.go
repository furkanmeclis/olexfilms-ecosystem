package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/repository"
	"github.com/google/uuid"
)

// ListSessions returns active refresh sessions; current is marked via access-token sid.
func (u *AuthUseCase) ListSessions(ctx context.Context, userID int64, current uuid.UUID) ([]model.DeviceSession, error) {
	items, err := u.repo.ListActiveSessions(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Current = current != uuid.Nil && items[i].UUID == current
	}
	return items, nil
}

// RevokeSession revokes one refresh session belonging to the user.
func (u *AuthUseCase) RevokeSession(ctx context.Context, userID int64, sessionUUID uuid.UUID) error {
	if sessionUUID == uuid.Nil {
		return fmt.Errorf("%w: session id is required", ErrInvalidRequest)
	}
	err := u.repo.RevokeSession(ctx, userID, sessionUUID)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrNotFound
	}
	if err == nil {
		u.RevokeAccessSession(ctx, sessionUUID)
	}
	return err
}

// RevokeOtherSessions keeps the current session and revokes the rest.
func (u *AuthUseCase) RevokeOtherSessions(ctx context.Context, userID int64, current uuid.UUID) error {
	if current == uuid.Nil {
		return fmt.Errorf("%w: current session is unknown", ErrInvalidRequest)
	}
	others, err := u.repo.ListActiveSessions(ctx, userID)
	if err != nil {
		return err
	}
	if err := u.repo.RevokeOtherSessions(ctx, userID, current); err != nil {
		return err
	}
	for _, s := range others {
		if s.UUID != current {
			u.RevokeAccessSession(ctx, s.UUID)
		}
	}
	return nil
}

// EndAccessSession finishes a logout for the session the access token belongs
// to. Without a refresh token (API clients, scripts) the refresh session is
// revoked too, so logout always ends it; the access token itself stops
// working now instead of at expiry.
func (u *AuthUseCase) EndAccessSession(ctx context.Context, rawRefresh string, userID int64, sid uuid.UUID) {
	if sid == uuid.Nil {
		return
	}
	if strings.TrimSpace(rawRefresh) == "" {
		_ = u.RevokeSession(ctx, userID, sid)
	}
	u.RevokeAccessSession(ctx, sid)
}

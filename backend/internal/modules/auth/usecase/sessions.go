package usecase

import (
	"context"
	"errors"
	"fmt"

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
	return err
}

// RevokeOtherSessions keeps the current session and revokes the rest.
func (u *AuthUseCase) RevokeOtherSessions(ctx context.Context, userID int64, current uuid.UUID) error {
	if current == uuid.Nil {
		return fmt.Errorf("%w: current session is unknown", ErrInvalidRequest)
	}
	return u.repo.RevokeOtherSessions(ctx, userID, current)
}

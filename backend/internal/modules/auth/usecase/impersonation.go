package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

// ImpersonatePlatformUser starts a session as another active user.
func (u *AuthUseCase) ImpersonatePlatformUser(
	ctx context.Context,
	actor model.User,
	actorIsSuperAdmin bool,
	alreadyImpersonating bool,
	targetUUID uuid.UUID,
	meta model.SessionMeta,
) (model.ImpersonationResult, error) {
	if err := requirePanelRealm(&meta); err != nil {
		return model.ImpersonationResult{}, err
	}
	if actor.UUID == targetUUID {
		return model.ImpersonationResult{}, fmt.Errorf("%w: cannot impersonate yourself", ErrInvalidRequest)
	}
	if alreadyImpersonating {
		return model.ImpersonationResult{}, ErrAlreadyImpersonating
	}

	target, err := u.repo.FindUserByUUID(ctx, targetUUID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.ImpersonationResult{}, ErrNotFound
		}
		return model.ImpersonationResult{}, err
	}
	if target.Status != "active" {
		return model.ImpersonationResult{}, fmt.Errorf("%w: only active users can be impersonated", ErrInvalidRequest)
	}

	targetIsSuperAdmin, err := u.repo.UserHasRoleSlug(ctx, target.ID, rbac.RoleSuperAdmin)
	if err != nil {
		return model.ImpersonationResult{}, err
	}
	if targetIsSuperAdmin && !actorIsSuperAdmin {
		return model.ImpersonationResult{}, fmt.Errorf("%w: cannot impersonate a super admin", ErrForbidden)
	}

	meta.ImpersonatorUserID = &actor.ID
	tokens, err := u.issueTokensForUser(ctx, target, meta, nil)
	if err != nil {
		return model.ImpersonationResult{}, err
	}

	return model.ImpersonationResult{
		Tokens: tokens,
		Session: model.SessionSwitch{
			UserUUID:         target.UUID,
			Email:            target.Email,
			ImpersonatorUUID: &actor.UUID,
		},
	}, nil
}

// StopImpersonation restores the original actor session.
func (u *AuthUseCase) StopImpersonation(
	ctx context.Context,
	impersonatorUUID uuid.UUID,
	meta model.SessionMeta,
) (model.ImpersonationResult, error) {
	if err := requirePanelRealm(&meta); err != nil {
		return model.ImpersonationResult{}, err
	}
	actor, err := u.repo.FindUserByUUID(ctx, impersonatorUUID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.ImpersonationResult{}, ErrNotFound
		}
		return model.ImpersonationResult{}, err
	}
	if actor.Status == "disabled" {
		return model.ImpersonationResult{}, ErrUserDisabled
	}
	if actor.Status != "active" {
		return model.ImpersonationResult{}, ErrUserDisabled
	}

	meta.ImpersonatorUserID = nil
	tokens, err := u.issueTokensForUser(ctx, actor, meta, nil)
	if err != nil {
		return model.ImpersonationResult{}, err
	}

	return model.ImpersonationResult{
		Tokens: tokens,
		Session: model.SessionSwitch{
			UserUUID: actor.UUID,
			Email:    actor.Email,
		},
	}, nil
}

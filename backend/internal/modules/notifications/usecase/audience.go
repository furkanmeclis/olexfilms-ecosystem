package usecase

import (
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	audienceMe  = "me"
	audienceAll = "all"
)

type platformAudience struct {
	UserID      pgtype.Int8
	IncludeUser bool
}

func resolvePlatformAudience(
	p authctx.Principal,
	scope string,
	rawUserUUID string,
	lookup func(uuid.UUID) (int64, error),
) (platformAudience, error) {
	scope = strings.ToLower(strings.TrimSpace(scope))
	rawUserUUID = strings.TrimSpace(rawUserUUID)
	canReadAll := p.HasPermission(rbac.PermPlatformNotificationsReadAll)

	if !canReadAll {
		return platformAudience{
			UserID: pgtype.Int8{Int64: p.UserInternal, Valid: true},
		}, nil
	}

	if rawUserUUID != "" {
		id, err := uuid.Parse(rawUserUUID)
		if err != nil {
			return platformAudience{}, ErrInvalidRequest
		}
		internalID, err := lookup(id)
		if err != nil {
			return platformAudience{}, err
		}
		return platformAudience{
			UserID:      pgtype.Int8{Int64: internalID, Valid: true},
			IncludeUser: true,
		}, nil
	}

	if scope == audienceAll {
		return platformAudience{IncludeUser: true}, nil
	}

	return platformAudience{
		UserID: pgtype.Int8{Int64: p.UserInternal, Valid: true},
	}, nil
}

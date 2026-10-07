package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/password"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// UserSort is the sort contract of the fleet user list.
var UserSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"name": "name", "email": "email", "status": "status", "created_at": "created_at",
	},
	Default: apiquery.SortField{Field: "created_at"},
}

// InviteInput is POST /v1/fleets/{uuid}/users.
type InviteInput struct {
	Email   string `json:"email"`
	Name    string `json:"name"`
	Surname string `json:"surname"`
}

// UserFilter narrows GET /v1/fleets/{uuid}/users.
type UserFilter struct {
	Q             string
	Statuses      []string
	Sort          []apiquery.SortField
	Limit, Offset int32
}

func normalizeInvite(in InviteInput) (InviteInput, error) {
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	addr, err := mail.ParseAddress(in.Email)
	if in.Email == "" || err != nil || addr.Address != in.Email || len(in.Email) > 255 {
		return in, &ValidationError{Field: "email", Message: "a valid e-mail address is required"}
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > 100 {
		return in, &ValidationError{Field: "name", Message: "required, at most 100 characters"}
	}
	in.Surname = strings.TrimSpace(in.Surname)
	if utf8.RuneCountInString(in.Surname) > 100 {
		return in, &ValidationError{Field: "surname", Message: "at most 100 characters"}
	}
	return in, nil
}

// InviteUser is POST /v1/fleets/{uuid}/users: a new account with the global
// fleet role (portal realm, e-mail + password) joins the fleet. The first
// user of a fleet is its primary user and owns the fleet vehicles; the
// dealers with an active link serve them as a customer. The password is set
// through the existing reset flow: a one-time code goes to the e-mail.
// An e-mail that already has an account is ErrUserEmailTaken (409).
func (s *Service) InviteUser(ctx context.Context, c Caller, fleetUUID uuid.UUID, in InviteInput) (FleetUserView, error) {
	in, err := normalizeInvite(in)
	if err != nil {
		return FleetUserView{}, err
	}
	hash, err := unusablePasswordHash()
	if err != nil {
		return FleetUserView{}, err
	}
	var out FleetUserView
	err = s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		a, err := s.resolve(ctx, q, c, fleetUUID)
		if err != nil {
			return err
		}
		fleet := a.fleet.Organization
		user, err := q.CreateUser(ctx, db.CreateUserParams{
			Email: pgtype.Text{String: in.Email, Valid: true}, PasswordHash: hash,
			Name: in.Name, Surname: in.Surname, Status: "active",
		})
		if isUnique(err, "uq_users_email_active") {
			return ErrUserEmailTaken
		}
		if err != nil {
			return fmt.Errorf("fleet: create user: %w", err)
		}
		if err := q.AssignUserRoleBySlug(ctx, db.AssignUserRoleBySlugParams{UserID: user.ID, Slug: rbac.RoleFleet}); err != nil {
			return fmt.Errorf("fleet: role: %w", err)
		}
		n, err := q.CountFleetUsersOfFleet(ctx, fleet.ID)
		if err != nil {
			return fmt.Errorf("fleet: count users: %w", err)
		}
		primary := n == 0
		fu, err := q.CreateFleetUser(ctx, db.CreateFleetUserParams{
			FleetOrgID: fleet.ID, BrandID: fleet.BrandID, UserID: user.ID, IsPrimary: primary,
			InvitedByOrgID:  pgtype.Int8{Int64: c.OrgID, Valid: c.OrgID != 0},
			InvitedByUserID: pgtype.Int8{Int64: c.UserID, Valid: c.UserID != 0},
		})
		if isUnique(err, "uq_fleet_users_primary") {
			// A concurrent invitation became the primary user first.
			return ErrConcurrentChange
		}
		if err != nil {
			return fmt.Errorf("fleet: fleet user: %w", err)
		}
		if primary {
			if err := s.setPrimary(ctx, q, fleet, user.ID); err != nil {
				return err
			}
		}
		inviter := ""
		if org, err := q.GetOrganizationByID(ctx, c.OrgID); err == nil {
			inviter = org.Name
		}
		if err := s.emit(ctx, tx, events.FleetUserInvited, c.OrgID, c.UserID, fleet, map[string]any{
			"user_uuid": user.Uuid.String(), "is_primary": primary, "dealer_name": inviter,
			"notify_user_ids": []int64{user.ID},
		}); err != nil {
			return err
		}
		out = FleetUserView{
			UUID: fu.Uuid, UserUUID: user.Uuid, Email: textPtr(user.Email), Name: user.Name, Surname: user.Surname,
			IsPrimary: fu.IsPrimary, Status: fu.Status, CreatedAt: fu.CreatedAt.Time,
		}
		return nil
	})
	if err != nil {
		return FleetUserView{}, err
	}
	// The existing password flow: a reset code by e-mail (a failed send is
	// recovered from the portal's forgot-password form).
	if s.inviter != nil {
		_ = s.inviter.ForgotPassword(ctx, in.Email)
	}
	return out, nil
}

// ErrConcurrentChange: a concurrent write won; the client retries (409).
var ErrConcurrentChange = &RuleError{
	Code: model.CodeLinkStale, Message: "the fleet changed concurrently, retry", status: http.StatusConflict,
}

// setPrimary records the vehicle owner on the profile and links it to every
// dealer with an active link (their customer flows reach the vehicles).
func (s *Service) setPrimary(ctx context.Context, q *db.Queries, fleet db.Organization, userID int64) error {
	if _, err := q.UpdateFleetProfile(ctx, db.UpdateFleetProfileParams{
		OrganizationID: fleet.ID, PrimaryUserID: pgtype.Int8{Int64: userID, Valid: true},
	}); err != nil {
		return fmt.Errorf("fleet: primary user: %w", err)
	}
	links, err := q.ListFleetDealerLinks(ctx, db.ListFleetDealerLinksParams{
		FleetOrgID: fleet.ID, Statuses: []string{model.LinkActive},
	})
	if err != nil {
		return fmt.Errorf("fleet: links: %w", err)
	}
	for _, l := range links {
		if err := repository.LinkPrimaryUser(ctx, q, userID, l.FleetDealerLink.DealerOrgID, fleet.BrandID); err != nil {
			return err
		}
	}
	return nil
}

// ListUsers is GET /v1/fleets/{uuid}/users.
func (s *Service) ListUsers(ctx context.Context, c Caller, fleetUUID uuid.UUID, f UserFilter) ([]FleetUserView, int64, error) {
	sort, err := apiquery.ResolveSort(f.Sort, UserSort)
	if err != nil {
		return nil, 0, err
	}
	q := db.New(s.conn)
	a, err := s.resolve(ctx, q, c, fleetUUID)
	if err != nil {
		return nil, 0, err
	}
	qText := pgtype.Text{String: strings.TrimSpace(f.Q), Valid: strings.TrimSpace(f.Q) != ""}
	rows, err := q.ListFleetUsers(ctx, db.ListFleetUsersParams{
		FleetOrgID: a.orgID(), Statuses: f.Statuses, Q: qText,
		SortKey: sort.Key, SortDesc: sort.Desc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("fleet: list users: %w", err)
	}
	total, err := q.CountFleetUsers(ctx, db.CountFleetUsersParams{FleetOrgID: a.orgID(), Statuses: f.Statuses, Q: qText})
	if err != nil {
		return nil, 0, fmt.Errorf("fleet: count users: %w", err)
	}
	out := make([]FleetUserView, 0, len(rows))
	for _, r := range rows {
		out = append(out, FleetUserView{
			UUID: r.Uuid, UserUUID: r.UserUuid, Email: textPtr(r.Email), Name: r.Name, Surname: r.Surname,
			IsPrimary: r.IsPrimary, Status: r.Status, LastLoginAt: timePtr(r.LastLoginAt),
			DisabledAt: timePtr(r.DisabledAt), CreatedAt: r.CreatedAt.Time,
		})
	}
	return out, total, nil
}

// DisableUser is POST /v1/fleets/{uuid}/users/{user_uuid}/disable: the
// fleet user can no longer sign in (users.status disabled, sessions
// revoked); the row stays as history. The primary user owns the fleet
// vehicles and is not disabled (ErrPrimaryUserLocked).
func (s *Service) DisableUser(ctx context.Context, c Caller, fleetUUID, id uuid.UUID) error {
	return s.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		a, err := s.resolve(ctx, q, c, fleetUUID)
		if err != nil {
			return err
		}
		fu, err := q.GetFleetUserByUUID(ctx, db.GetFleetUserByUUIDParams{Uuid: id, FleetOrgID: a.orgID()})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("fleet: user: %w", err)
		}
		if fu.IsPrimary {
			return ErrPrimaryUserLocked
		}
		if fu.Status == model.UserDisabled {
			return nil
		}
		if _, err := q.DisableFleetUser(ctx, fu.ID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("fleet: disable: %w", err)
		}
		user, err := q.GetUserByID(ctx, fu.UserID)
		if err != nil {
			return fmt.Errorf("fleet: user: %w", err)
		}
		if _, err := q.UpdateUserPlatform(ctx, db.UpdateUserPlatformParams{
			Uuid: user.Uuid, Status: pgtype.Text{String: "disabled", Valid: true},
		}); err != nil {
			return fmt.Errorf("fleet: user status: %w", err)
		}
		if err := q.RevokeAllRefreshTokensForUser(ctx, user.ID); err != nil {
			return fmt.Errorf("fleet: sessions: %w", err)
		}
		return nil
	})
}

// unusablePasswordHash is a random password nobody knows: the user sets
// their own through the reset code (same rule as the customers module).
func unusablePasswordHash() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return password.Hash("Aa1" + base64.RawStdEncoding.EncodeToString(b))
}

func isUnique(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}

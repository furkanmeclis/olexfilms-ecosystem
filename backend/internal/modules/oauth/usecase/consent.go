package usecase

// TEC-401 (F4-03b): the /oauth/authorize endpoint, the consent screen data
// and decision, with the realm rules of the MCP endpoints:
//
//   - /mcp/customer: portal (customer realm) sessions only; the token is
//     bound to the brand center (customer data spans the brand's dealers and
//     customer AI usage is the center's system pool).
//   - /mcp/dealer: panel users, only a distributor or dealer organization
//     they are a member of and hold mcp.connect in.
//   - /mcp/user: every panel user (center included) in an organization they
//     are a member of and hold mcp.connect in.
//
// The token is bound to the one organization chosen on consent. Every
// decision is written to the activity log in the decision's transaction.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrNotFound: the request, grant or client is missing, expired, foreign or
// already decided (404).
var ErrNotFound = errors.New("oauth: not found")

// ForbiddenError is a consent the caller may not give (403).
type ForbiddenError struct{ Message string }

func (e *ForbiddenError) Error() string { return "oauth: forbidden: " + e.Message }

func forbidden(msg string) error { return &ForbiddenError{Message: msg} }

func invalid(field, msg string) error {
	return &apiquery.ValidationError{Details: []apiquery.Detail{{Field: field, Message: msg}}}
}

// AccessChecker resolves the caller's permissions inside one organization
// (auth usecase ResolveAccess: global roles + the member roles there).
type AccessChecker interface {
	OrgPermission(ctx context.Context, userID int64, globalRoles []string, orgUUID uuid.UUID, perm string) (bool, error)
}

// SetAccess attaches the permission resolver of the consent decision.
func (s *Service) SetAccess(a AccessChecker) { s.access = a }

// Actor is the signed-in user deciding on consent or managing grants.
type Actor struct {
	UserID int64
	// Realm is the session audience (jwt.AudiencePanel / AudiencePortal).
	Realm        string
	Roles        []string
	IsSuperAdmin bool
	// BrandID is the request brand (brandctx); the customer realm binds to
	// its center.
	BrandID int64
	Meta    activity.Meta
}

func (a Actor) id() *int64 { id := a.UserID; return &id }

// --- authorize endpoint ------------------------------------------------------------

// Authorize serves GET /oauth/authorize: it applies the per-IP limit,
// validates and stores the request and returns where to send the browser:
// the consent screen, or (for errors after client and redirect_uri are
// verified) the client's redirect_uri with error, state and iss. An unknown
// client, an unregistered redirect_uri or the rate limit is returned as a
// *model.Error that must be shown to the user, never redirected.
func (s *Service) Authorize(ctx context.Context, ip string, in model.AuthorizeInput) (string, error) {
	if err := s.AllowAuthorize(ctx, ip); err != nil {
		return "", err
	}
	req, err := s.CreateAuthRequest(ctx, in)
	if err != nil {
		var oe *model.Error
		if errors.As(err, &oe) {
			if !oe.Redirectable {
				return "", oe
			}
			return s.errorRedirect(in.RedirectURI, in.State, oe.Code, oe.Description)
		}
		return "", serverError(err)
	}
	return s.issuer + model.ConsentPath + "?request=" + req.ID.String(), nil
}

// errorRedirect builds the client redirect of an authorization error (RFC
// 6749 §4.1.2.1, iss per RFC 9207).
func (s *Service) errorRedirect(redirectURI, state, code, desc string) (string, error) {
	u, err := url.Parse(redirectURI)
	if err != nil {
		return "", serverError(err)
	}
	v := u.Query()
	v.Set("error", code)
	if desc != "" {
		v.Set("error_description", desc)
	}
	if state != "" {
		v.Set("state", state)
	}
	v.Set("iss", s.issuer)
	u.RawQuery = v.Encode()
	return u.String(), nil
}

// --- consent ------------------------------------------------------------------------

// checkRealm enforces who may decide on a request for resource.
func checkRealm(a Actor, resource string) error {
	portal := jwt.NormalizeAudience(a.Realm) == jwt.AudiencePortal
	switch model.RealmOf(resource) {
	case model.RealmCustomer:
		if !portal {
			return forbidden("the customer MCP endpoint is only for customer portal accounts")
		}
	default:
		if jwt.NormalizeAudience(a.Realm) != jwt.AudiencePanel {
			return forbidden("this MCP endpoint is only for panel accounts")
		}
	}
	return nil
}

// orgTypes are the organization types each panel realm may bind to.
func orgTypes(realm string) []string {
	if realm == model.RealmDealer {
		return []string{rbac.OrgTypeDistributor, rbac.OrgTypeDealer}
	}
	return []string{rbac.OrgTypePlatform, rbac.OrgTypeCenter, rbac.OrgTypeDistributor, rbac.OrgTypeDealer}
}

type selectableOrg struct {
	model.ConsentOrganization
	id, brandID int64
}

// selectable lists the organizations the actor may bind a token for
// resource to: realm, membership, mcp.connect there and the mcp module on.
func (s *Service) selectable(ctx context.Context, a Actor, resource string) ([]selectableOrg, error) {
	realm := model.RealmOf(resource)
	var cands []selectableOrg
	add := func(id int64, uid uuid.UUID, name, typ string, brandID int64) {
		for _, c := range cands {
			if c.id == id {
				return
			}
		}
		cands = append(cands, selectableOrg{ConsentOrganization: model.ConsentOrganization{UUID: uid, Name: name, Type: typ}, id: id, brandID: brandID})
	}
	center := func() error {
		if a.BrandID == 0 {
			return nil
		}
		c, err := s.q.GetOAuthBrandCenter(ctx, a.BrandID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		add(c.ID, c.Uuid, c.Name, c.Type, c.BrandID)
		return nil
	}
	if realm == model.RealmCustomer {
		if err := center(); err != nil {
			return nil, fmt.Errorf("oauth: brand center: %w", err)
		}
	} else {
		rows, err := s.q.ListOAuthSelectableOrgs(ctx, db.ListOAuthSelectableOrgsParams{UserID: a.UserID, Types: orgTypes(realm)})
		if err != nil {
			return nil, fmt.Errorf("oauth: organizations: %w", err)
		}
		for _, r := range rows {
			add(r.ID, r.Uuid, r.Name, r.Type, r.BrandID)
		}
		// The platform admin acts for the center without a membership.
		if realm == model.RealmUser && a.IsSuperAdmin {
			if err := center(); err != nil {
				return nil, fmt.Errorf("oauth: brand center: %w", err)
			}
		}
	}
	out := make([]selectableOrg, 0, len(cands))
	for _, c := range cands {
		if realm != model.RealmCustomer {
			if s.access == nil {
				return nil, errors.New("oauth: access checker missing")
			}
			ok, err := s.access.OrgPermission(ctx, a.UserID, a.Roles, c.UUID, rbac.PermMCPConnect)
			if err != nil {
				return nil, fmt.Errorf("oauth: access: %w", err)
			}
			if !ok {
				continue
			}
		}
		on, err := s.mcpEnabled(ctx, c.id)
		if err != nil {
			return nil, err
		}
		if on {
			out = append(out, c)
		}
	}
	return out, nil
}

// Consent returns the consent screen data of a pending request: client,
// redirect host, resource and the organizations the caller may choose.
func (s *Service) Consent(ctx context.Context, a Actor, id uuid.UUID) (model.Consent, error) {
	req, err := s.GetAuthRequest(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Consent{}, ErrNotFound
	}
	if err != nil {
		return model.Consent{}, err
	}
	if err := checkRealm(a, req.Resource); err != nil {
		return model.Consent{}, err
	}
	orgs, err := s.selectable(ctx, a, req.Resource)
	if err != nil {
		return model.Consent{}, err
	}
	host := req.RedirectURI
	if u, err := url.Parse(req.RedirectURI); err == nil {
		host = u.Host
	}
	out := model.Consent{
		RequestID: req.ID, ClientID: req.ClientID, ClientName: req.ClientName, RedirectHost: host,
		Resource: req.Resource, ResourceURL: s.ResourceURL(req.Resource), Realm: model.RealmOf(req.Resource),
		Organizations: make([]model.ConsentOrganization, 0, len(orgs)), ExpiresAt: req.ExpiresAt,
	}
	for _, o := range orgs {
		out.Organizations = append(out.Organizations, o.ConsentOrganization)
	}
	return out, nil
}

// Decide approves (binding the token to the chosen organization) or denies
// a pending request and returns the client redirect: the code on approval,
// error=access_denied on denial. The request is consumed either way.
func (s *Service) Decide(ctx context.Context, a Actor, id uuid.UUID, in model.DecideInput) (model.Decision, error) {
	if in.Decision != model.DecisionApprove && in.Decision != model.DecisionDeny {
		return model.Decision{}, invalid("decision", "must be approve or deny")
	}
	req, err := s.GetAuthRequest(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Decision{}, ErrNotFound
	}
	if err != nil {
		return model.Decision{}, err
	}
	if err := checkRealm(a, req.Resource); err != nil {
		return model.Decision{}, err
	}
	payload := map[string]any{"client_id": req.ClientID, "client_name": req.ClientName, "resource": req.Resource}
	if in.Decision == model.DecisionDeny {
		return s.deny(ctx, a, req, payload)
	}

	realm := model.RealmOf(req.Resource)
	if realm != model.RealmCustomer && in.OrganizationUUID == nil {
		return model.Decision{}, invalid("organization_uuid", "required to approve")
	}
	orgs, err := s.selectable(ctx, a, req.Resource)
	if err != nil {
		return model.Decision{}, err
	}
	var chosen *selectableOrg
	for i := range orgs {
		if in.OrganizationUUID == nil || orgs[i].UUID == *in.OrganizationUUID {
			chosen = &orgs[i]
			break
		}
	}
	if chosen == nil {
		return model.Decision{}, forbidden("the organization cannot be connected: no membership, no mcp.connect permission or the mcp module is off")
	}
	payload["organization_uuid"] = chosen.UUID.String()
	payload["organization_name"] = chosen.Name
	redirect, err := s.issueCode(ctx, model.IssueCodeInput{
		RequestID: req.ID, UserID: a.UserID, OrganizationID: chosen.id, BrandID: chosen.brandID,
	}, func(q *db.Queries, grant db.OauthGrant) error {
		g := grant.Uuid
		return activity.Write(ctx, q, a.id(), model.ActionConsentApproved, "oauth_grants", &g, payload, a.Meta)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Decision{}, ErrNotFound
	}
	var oe *model.Error
	if errors.As(err, &oe) && oe.Status == http.StatusForbidden {
		return model.Decision{}, forbidden(oe.Description)
	}
	if err != nil {
		return model.Decision{}, err
	}
	return model.Decision{RedirectURL: redirect}, nil
}

func (s *Service) deny(ctx context.Context, a Actor, req model.AuthRequest, payload map[string]any) (model.Decision, error) {
	err := s.inTx(ctx, func(q *db.Queries) error {
		n, err := q.DeleteOAuthAuthRequest(ctx, req.ID)
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		id := req.ID
		return activity.Write(ctx, q, a.id(), model.ActionConsentDenied, "oauth_auth_requests", &id, payload, a.Meta)
	})
	if err != nil {
		return model.Decision{}, err
	}
	redirect, err := s.errorRedirect(req.RedirectURI, req.State, model.ErrAccessDenied, "the user denied access")
	if err != nil {
		return model.Decision{}, err
	}
	return model.Decision{RedirectURL: redirect}, nil
}

// --- connected apps -------------------------------------------------------------------

// GrantsSortSpec is the sort whitelist of GET /v1/oauth/grants.
var GrantsSortSpec = apiquery.SortSpec{
	Columns: apiquery.SortColumns{"created_at": "created_at", "last_used_at": "last_used_at", "client_name": "client_name"},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

// ClientsSortSpec is the sort whitelist of GET /v1/platform/oauth/clients.
var ClientsSortSpec = apiquery.SortSpec{
	Columns: apiquery.SortColumns{"created_at": "created_at", "last_used_at": "last_used_at", "client_name": "client_name"},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

// Realms is the filter domain of the grant list (realm=customer,dealer,user).
var Realms = []string{model.RealmCustomer, model.RealmDealer, model.RealmUser}

// ListFilter is a parsed list query (grants: realm filter; clients: status).
type ListFilter struct {
	Q        string
	Realms   []string
	Statuses []string
	Sort     apiquery.ResolvedSort
	Limit    int32
	Offset   int32
}

func parseList(values url.Values, spec apiquery.SortSpec) (ListFilter, error) {
	q := apiquery.Parse(values)
	f := ListFilter{Q: strings.TrimSpace(q.Q), Limit: q.Limit, Offset: q.Offset}
	var err error
	f.Sort, err = apiquery.ResolveSort(q.Sort, spec)
	return f, err
}

// ParseGrantListFilter reads q, realm (CSV), sort, limit and offset.
func ParseGrantListFilter(values url.Values) (ListFilter, error) {
	f, err := parseList(values, GrantsSortSpec)
	if err != nil {
		return f, err
	}
	f.Realms, err = apiquery.EnumList(values, "realm", Realms...)
	return f, err
}

// ParseClientListFilter reads q, status (CSV active,revoked), sort, limit
// and offset.
func ParseClientListFilter(values url.Values) (ListFilter, error) {
	f, err := parseList(values, ClientsSortSpec)
	if err != nil {
		return f, err
	}
	f.Statuses, err = apiquery.EnumList(values, "status", model.ClientStatusActive, model.ClientStatusRevoked)
	return f, err
}

func textNarg(s string) (t pgtype.Text) {
	if s != "" {
		t.String, t.Valid = s, true
	}
	return t
}

// ListGrants lists the user's connected apps (live grants of live clients).
func (s *Service) ListGrants(ctx context.Context, userID int64, f ListFilter) ([]model.Grant, int64, error) {
	var resources []string
	for _, r := range f.Realms {
		resources = append(resources, "/mcp/"+r)
	}
	rows, err := s.q.ListUserOAuthGrants(ctx, db.ListUserOAuthGrantsParams{
		UserID: userID, Resources: resources, Q: textNarg(f.Q),
		SortKey: f.Sort.Key, SortDesc: f.Sort.Desc, RowLimit: f.Limit, RowOffset: f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("oauth: grants: %w", err)
	}
	total, err := s.q.CountUserOAuthGrants(ctx, db.CountUserOAuthGrantsParams{UserID: userID, Resources: resources, Q: textNarg(f.Q)})
	if err != nil {
		return nil, 0, fmt.Errorf("oauth: grants count: %w", err)
	}
	out := make([]model.Grant, 0, len(rows))
	for _, r := range rows {
		out = append(out, model.Grant{
			UUID: r.Uuid, ClientID: r.ClientID, ClientName: r.ClientName, Resource: r.Resource,
			Realm: model.RealmOf(r.Resource), Scopes: r.Scopes, OrganizationUUID: r.OrganizationUuid,
			OrganizationName: r.OrganizationName, OrganizationType: r.OrganizationType,
			CreatedAt: r.CreatedAt.Time, LastUsedAt: timePtr(r.LastUsedAt),
		})
	}
	return out, total, nil
}

// RevokeGrant disconnects one of the actor's apps: the grant and every
// token family issued under it (its access tokens answer 401 at once).
func (s *Service) RevokeGrant(ctx context.Context, a Actor, id uuid.UUID) error {
	return s.inTx(ctx, func(q *db.Queries) error {
		g, err := q.RevokeUserOAuthGrant(ctx, db.RevokeUserOAuthGrantParams{Uuid: id, UserID: a.UserID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := q.RevokeOAuthGrantTokens(ctx, g.ID); err != nil {
			return err
		}
		return activity.Write(ctx, q, a.id(), model.ActionGrantRevoked, "oauth_grants", &g.Uuid,
			map[string]any{"client_id": g.ClientID, "client_name": g.ClientName, "resource": g.Resource}, a.Meta)
	})
}

// --- platform clients -----------------------------------------------------------------

// ListClients lists every registered client (platform admin).
func (s *Service) ListClients(ctx context.Context, f ListFilter) ([]model.ClientSummary, int64, error) {
	var status pgtype.Text
	if len(f.Statuses) == 1 {
		status = textNarg(f.Statuses[0])
	}
	rows, err := s.q.ListOAuthClients(ctx, db.ListOAuthClientsParams{
		Status: status, Q: textNarg(f.Q),
		SortKey: f.Sort.Key, SortDesc: f.Sort.Desc, RowLimit: f.Limit, RowOffset: f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("oauth: clients: %w", err)
	}
	total, err := s.q.CountOAuthClients(ctx, db.CountOAuthClientsParams{Status: status, Q: textNarg(f.Q)})
	if err != nil {
		return nil, 0, fmt.Errorf("oauth: clients count: %w", err)
	}
	out := make([]model.ClientSummary, 0, len(rows))
	for _, r := range rows {
		c := model.ClientSummary{
			UUID: r.Uuid, ClientID: r.ClientID, ClientName: r.ClientName, RedirectURIs: r.RedirectUris,
			Status: model.ClientStatusActive, ActiveGrants: r.ActiveGrants, CreatedAt: r.CreatedAt.Time,
			LastUsedAt: timePtr(r.LastUsedAt), RevokedAt: timePtr(r.RevokedAt),
		}
		if r.CreatedIp.Valid {
			ip := r.CreatedIp.String
			c.CreatedIP = &ip
		}
		if r.RevokedAt.Valid {
			c.Status = model.ClientStatusRevoked
		}
		if c.RedirectURIs == nil {
			c.RedirectURIs = []string{}
		}
		out = append(out, c)
	}
	return out, total, nil
}

// RevokeClient blocks a client and everything issued to it: grants, every
// token, pending requests and unused codes. Revoking a revoked client again
// is a no-op that still answers success.
func (s *Service) RevokeClient(ctx context.Context, a Actor, id uuid.UUID) error {
	return s.inTx(ctx, func(q *db.Queries) error {
		c, err := q.GetOAuthClientByUUID(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := q.RevokeOAuthClient(ctx, c.ClientID); err != nil {
			return err
		}
		return activity.Write(ctx, q, a.id(), model.ActionClientRevoked, "oauth_clients", &c.Uuid,
			map[string]any{"client_id": c.ClientID, "client_name": c.ClientName, "already_revoked": c.RevokedAt.Valid}, a.Meta)
	})
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

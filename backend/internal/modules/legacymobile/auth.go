package legacymobile

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// Old role codes (olexfilms UserRoleEnum, docs/mobile-api.md "Roller").
const (
	roleSuperAdmin  = "super_admin"
	roleCenterStaff = "center_staff"
	roleDealerOwner = "dealer_owner"
	roleDealerStaff = "dealer_staff"
)

// newMe is the part of the new mobile me (GET /v1/mobile/auth/me, and "me"
// in the login answer) the old user is built from.
type newMe struct {
	User struct {
		UUID         uuid.UUID `json:"uuid"`
		Email        string    `json:"email"`
		Name         string    `json:"name"`
		Surname      string    `json:"surname"`
		IsSuperAdmin bool      `json:"is_super_admin"`
	} `json:"user"`
	ActiveOrganization *uuid.UUID `json:"active_organization_uuid"`
	Organizations      []newOrg   `json:"organizations"`
	EffectiveLocale    string     `json:"effective_locale"`
}

type newOrg struct {
	UUID   uuid.UUID `json:"uuid"`
	Slug   string    `json:"slug"`
	Name   string    `json:"name"`
	Role   string    `json:"role"`
	Status string    `json:"status"`
	Type   string    `json:"type"`
}

func (m newMe) active() *newOrg {
	if m.ActiveOrganization == nil {
		return nil
	}
	for i := range m.Organizations {
		if m.Organizations[i].UUID == *m.ActiveOrganization {
			return &m.Organizations[i]
		}
	}
	return nil
}

// legacyDealerRef is UserResource.dealer. The hub always loaded the
// relation, so a user without a dealer (admin, center staff) has the key
// with null members.
type legacyDealerRef struct {
	ID       *int64  `json:"id"`
	Name     *string `json:"name"`
	IsActive *bool   `json:"is_active"`
}

// legacyUser is the hub's App\Http\Resources\Api\UserResource. uuid is
// additive (the new id of the user).
type legacyUser struct {
	ID        int64           `json:"id"`
	UUID      uuid.UUID       `json:"uuid"`
	Name      string          `json:"name"`
	Email     *string         `json:"email"`
	Phone     *string         `json:"phone"`
	Locale    string          `json:"locale"`
	AvatarURL *string         `json:"avatar_url"`
	DealerID  *int64          `json:"dealer_id"`
	Roles     []string        `json:"roles"`
	Dealer    legacyDealerRef `json:"dealer"`
}

// legacyRoles maps the new identity onto the hub's four mobile roles: the
// platform super admin is super_admin; in the active organization a center
// member is center_staff, a dealer (or distributor) owner dealer_owner and
// any other member dealer_staff.
func legacyRoles(m newMe) []string {
	if m.User.IsSuperAdmin {
		return []string{roleSuperAdmin}
	}
	org := m.active()
	switch {
	case org == nil:
		return []string{}
	case org.Type == "center":
		return []string{roleCenterStaff}
	case org.Role == "owner":
		return []string{roleDealerOwner}
	default:
		return []string{roleDealerStaff}
	}
}

// legacyLocaleCode writes a locale the way the hub stored it (zh_CN).
func legacyLocaleCode(l string) string {
	if l == "" {
		return "tr"
	}
	return strings.ReplaceAll(l, "-", "_")
}

func (a *adapters) legacyUser(ctx context.Context, m newMe) legacyUser {
	name := strings.TrimSpace(m.User.Name + " " + m.User.Surname)
	out := legacyUser{
		UUID: m.User.UUID, Name: name, Locale: legacyLocaleCode(m.EffectiveLocale),
		Roles: legacyRoles(m),
	}
	if m.User.Email != "" {
		e := m.User.Email
		out.Email = &e
	}
	if a.store != nil {
		if u, err := a.store.GetUserByUUID(ctx, m.User.UUID); err == nil {
			out.ID = u.ID
			if u.PhoneE164.Valid && u.PhoneE164.String != "" {
				p := u.PhoneE164.String
				out.Phone = &p
			}
		}
	}
	if org := m.active(); org != nil && org.Type != "center" {
		n, active := org.Name, org.Status == "active"
		out.Dealer = legacyDealerRef{Name: &n, IsActive: &active}
		if a.store != nil {
			if o, err := a.store.GetOrganizationByUUID(ctx, org.UUID); err == nil {
				id := o.ID
				out.DealerID, out.Dealer.ID = &id, &id
			}
		}
	}
	return out
}

// pickOrganization chooses the organization of a legacy login, which has
// no organization_slug: the only membership, otherwise the first active
// center membership, otherwise the first active one. "" keeps the session
// without an organization (a platform super admin without memberships).
func pickOrganization(orgs []newOrg) string {
	if len(orgs) == 1 {
		return orgs[0].Slug
	}
	for _, o := range orgs {
		if o.Type == "center" && o.Status == "active" {
			return o.Slug
		}
	}
	for _, o := range orgs {
		if o.Status == "active" {
			return o.Slug
		}
	}
	return ""
}

// legacyLoginInput is the hub's LoginRequest (email, password, optional
// device_name). organization_slug and totp_code are additive: an app that
// knows them may send them.
type legacyLoginInput struct {
	Email            string
	Password         string
	DeviceName       string
	OrganizationSlug string
	TOTPCode         string
}

// login is POST {Prefix}/auth/login: AuthController::login.
//
//	request  {email, password, device_name?}
//	response {success, message, data: {token, token_type: "Bearer", user}}
//
// The new login needs a device (id, platform) and the organization; the
// old body has neither. Every legacy login gets its own device id (the hub
// issued one Sanctum token per login and never ended the others), the
// platform comes from the User-Agent, and the organization is picked from
// the memberships (pickOrganization) with the organization switch.
func (a *adapters) login(w http.ResponseWriter, r *http.Request) {
	obj, okBody := readObject(r)
	locale := legacyLocale(r)
	if !okBody {
		validationFailed(w, "email", message(locale, msgRequired))
		return
	}
	in := legacyLoginInput{
		Email: str(obj, "email"), Password: str(obj, "password"), DeviceName: str(obj, "device_name"),
		OrganizationSlug: str(obj, "organization_slug"), TOTPCode: str(obj, "totp_code"),
	}
	switch {
	case in.Email == "":
		validationFailed(w, "email", message(locale, msgRequired))
		return
	case !strings.Contains(in.Email, "@"):
		validationFailed(w, "email", message(locale, msgEmail))
		return
	case in.Password == "":
		validationFailed(w, "password", message(locale, msgRequired))
		return
	}
	deviceName := in.DeviceName
	if deviceName == "" {
		deviceName = "mobile" // MobileApi::TOKEN_NAME
	}
	if len(deviceName) > 128 {
		deviceName = deviceName[:128]
	}
	body := map[string]any{
		"email": in.Email, "password": in.Password, "totp_code": in.TOTPCode,
		"organization_slug": in.OrganizationSlug,
		"device": map[string]string{
			"id": "legacy-" + uuid.NewString(), "name": deviceName, "platform": platformFromUA(r.UserAgent()),
		},
	}
	c := run(a.h.Login, withJSON(r, body))
	var out struct {
		AccessToken string `json:"access_token"`
		Me          newMe  `json:"me"`
	}
	if !ok(c, &out) {
		passThrough(w, c)
		return
	}
	token, me := out.AccessToken, out.Me
	if me.ActiveOrganization == nil && a.h.SwitchOrganization != nil && a.authn != nil {
		if slug := pickOrganization(me.Organizations); slug != "" {
			c := run(middleware.Chain(a.h.SwitchOrganization, a.authn),
				withBearer(r, http.MethodPost, token, map[string]string{"organization_slug": slug}))
			var switched struct {
				AccessToken string `json:"access_token"`
			}
			if !ok(c, &switched) {
				passThrough(w, c)
				return
			}
			token = switched.AccessToken
			c = run(middleware.Chain(a.h.Me, a.authn), withBearer(r, http.MethodGet, token, nil))
			if !ok(c, &me) {
				passThrough(w, c)
				return
			}
		}
	}
	// The old app never refreshes: answer the long-lived legacy token of
	// the final session (after the organization switch).
	if a.h.Sessions != nil {
		legacy, err := a.h.Sessions.IssueLegacyMobileAccess(r.Context(), token)
		if err != nil {
			response.InternalErr(w, r, err, "failed to issue the legacy mobile token")
			return
		}
		token = legacy
	}
	writeSuccess(w, http.StatusOK, message(locale, msgLoginSuccess), map[string]any{
		"token": token, "token_type": "Bearer", "user": a.legacyUser(r.Context(), me),
	})
}

// logout is POST {Prefix}/auth/logout: AuthController::logout. It ends the
// device session of the token (the legacy token stops working at once) and
// drops the device's push tokens; the answer is data null.
func (a *adapters) logout(w http.ResponseWriter, r *http.Request) {
	c := run(a.h.Logout, r)
	if !ok(c, nil) {
		passThrough(w, c)
		return
	}
	writeSuccess(w, http.StatusOK, message(legacyLocale(r), msgLogoutSuccess), nil)
}

// me is GET {Prefix}/auth/me: AuthController::me, data = UserResource.
func (a *adapters) me(w http.ResponseWriter, r *http.Request) {
	c := run(a.h.Me, r)
	var me newMe
	if !ok(c, &me) {
		passThrough(w, c)
		return
	}
	writeSuccess(w, http.StatusOK, "", a.legacyUser(r.Context(), me))
}

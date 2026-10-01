package model

import (
	"time"

	"github.com/google/uuid"
)

// User is the domain identity.
type User struct {
	ID            int64
	UUID          uuid.UUID
	Email         string // empty for phone-only (customer) accounts
	Phone         string // E.164, empty when unset
	PasswordHash  string
	Name          string
	Surname       string
	Status        string
	Locale        string
	EmailVerified bool
	CreatedAt     time.Time
}

// UserAuthMethod is a login option linked to a platform user.
type UserAuthMethod struct {
	Kind     string    `json:"kind"` // password | passkey | oauth
	Provider *string   `json:"provider,omitempty"`
	Label    *string   `json:"label,omitempty"`
	LinkedAt time.Time `json:"linked_at"`
}

// RoleSummary is a lightweight role view for API responses.
type RoleSummary struct {
	UUID        uuid.UUID `json:"uuid"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description *string   `json:"description,omitempty"`
	IsSystem    bool      `json:"is_system"`
	// OrgType is platform, center, distributor, dealer, customer or fleet
	// (empty for custom roles, which are global).
	OrgType *string `json:"org_type,omitempty"`
}

// PermissionSummary is a catalog permission entry.
type PermissionSummary struct {
	UUID           uuid.UUID `json:"uuid"`
	Name           string    `json:"name"`
	Slug           string    `json:"slug"`
	Module         string    `json:"module"`
	Scopes         []string  `json:"scopes"`
	IsSensitive    bool      `json:"is_sensitive"`
	SuperAdminOnly bool      `json:"super_admin_only"`
	Description    *string   `json:"description,omitempty"`
}

// RoleGrant is one permission of a role with its scope.
type RoleGrant struct {
	Permission string `json:"permission"`
	Scope      string `json:"scope"`
}

// RoleDetail includes permission slugs.
type RoleDetail struct {
	RoleSummary
	PermissionSlugs []string    `json:"permission_slugs"`
	Grants          []RoleGrant `json:"grants"`
}

// Grant is one permission granted with a scope by a role.
type Grant struct {
	Role       string `json:"role"`
	Permission string `json:"permission"`
	Scope      string `json:"scope"`
}

// Tokens is the auth token pair returned to clients.
type Tokens struct {
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token"`
	TokenType        string    `json:"token_type"`
	ExpiresIn        int64     `json:"expires_in"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
}

// MeLinks maps self-service paths for clients.
type MeLinks struct {
	Profile                 string `json:"profile"`
	ChangePassword          string `json:"change_password"`
	NotificationPreferences string `json:"notification_preferences"`
}

// MeChannels hints realtime subscribe channels.
type MeChannels struct {
	User string `json:"user"`
}

// MeRealtime is non-secret Centrifugo connection metadata for clients.
type MeRealtime struct {
	Enabled     bool   `json:"enabled"`
	WSURL       string `json:"ws_url,omitempty"`
	UserChannel string `json:"user_channel"`
}

// MeImpersonation describes an active impersonation session for the client.
type MeImpersonation struct {
	User PublicUser `json:"user"`
}

// Me is the session hydration payload.
type Me struct {
	User        PublicUser `json:"user"`
	Roles       []string   `json:"roles"`
	Permissions []string   `json:"permissions"`
	// Grants maps each held permission to its scope in the active
	// organization context (global roles + active membership roles).
	Grants map[string]string `json:"grants"`
	// ActiveOrganization is the organization of the session (JWT oid).
	ActiveOrganization *uuid.UUID `json:"active_organization_uuid,omitempty"`
	// OrganizationRoles are the membership roles in the active organization.
	OrganizationRoles []string              `json:"organization_roles"`
	Organizations     []OrganizationSummary `json:"organizations"`
	Links             MeLinks               `json:"links"`
	Channels          MeChannels            `json:"channels"`
	Realtime          MeRealtime            `json:"realtime"`
	Impersonation     *MeImpersonation      `json:"impersonation,omitempty"`
}

// OrganizationSummary is a tenant membership on /auth/me.
type OrganizationSummary struct {
	UUID         uuid.UUID  `json:"uuid"`
	Slug         string     `json:"slug"`
	Name         string     `json:"name"`
	Role         string     `json:"role"`
	LogoURL      *string    `json:"logo_url,omitempty"`
	Status       string     `json:"status"`
	AccessEndsAt *time.Time `json:"access_ends_at,omitempty"`
	// Type is center, distributor or dealer.
	Type   string                     `json:"type"`
	Brand  OrganizationSummaryBrand   `json:"brand"`
	Parent *OrganizationSummaryParent `json:"parent,omitempty"`
}

// OrganizationSummaryBrand is the brand of a membership.
type OrganizationSummaryBrand struct {
	Slug string `json:"slug"`
	Name string `json:"name,omitempty"`
}

// OrganizationSummaryParent is the parent (supplier) of a membership.
type OrganizationSummaryParent struct {
	UUID uuid.UUID `json:"uuid"`
	Slug string    `json:"slug,omitempty"`
	Name string    `json:"name"`
}

// PublicUser is the safe user projection.
type PublicUser struct {
	UUID          uuid.UUID `json:"uuid"`
	Email         string    `json:"email"`
	Name          string    `json:"name"`
	Surname       string    `json:"surname"`
	Status        string    `json:"status"`
	Locale        string    `json:"locale"`
	IsSuperAdmin  bool      `json:"is_super_admin"`
	EmailVerified bool      `json:"email_verified"`
}

// PlatformUserDetail is a platform user with assigned roles and login methods.
type PlatformUserDetail struct {
	PublicUser
	Roles       []RoleSummary    `json:"roles"`
	AuthMethods []UserAuthMethod `json:"auth_methods"`
}

// RegisterInput is self-serve user-only signup (no roles/tokens).
type RegisterInput struct {
	Email    string
	Password string
	Name     string
	Surname  string
}

// CreatePlatformUserInput creates a platform-managed user.
type CreatePlatformUserInput struct {
	Email     string
	Password  string
	Name      string
	Surname   string
	Status    string
	RoleUUIDs []uuid.UUID
}

// PatchPlatformUserInput partially updates a platform user.
type PatchPlatformUserInput struct {
	Name      *string
	Surname   *string
	Status    *string
	RoleUUIDs *[]uuid.UUID
}

// CreateRoleInput creates a custom role.
type CreateRoleInput struct {
	Name            string
	Slug            string
	Description     *string
	PermissionSlugs []string
	// Grants maps permission slug to scope; slugs only in PermissionSlugs get
	// the broadest scope the permission allows.
	Grants map[string]string
}

// PatchRoleInput partially updates a role.
type PatchRoleInput struct {
	Name            *string
	Description     *string
	PermissionSlugs *[]string
	Grants          *map[string]string
}

// RefreshSession is the persisted refresh token identity.
type RefreshSession struct {
	UUID               uuid.UUID
	UserID             int64
	ImpersonatorUserID *int64
	// OrganizationUUID is the tenant scope stamped on the prior access token, if any.
	OrganizationUUID *uuid.UUID
}

// DeviceSession is a user-visible refresh session (token never included).
type DeviceSession struct {
	UUID         uuid.UUID `json:"uuid"`
	Current      bool      `json:"current"`
	UserAgent    *string   `json:"user_agent,omitempty"`
	IPAddress    *string   `json:"ip_address,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	Impersonated bool      `json:"impersonated"`
}

// SessionMeta carries optional request metadata for refresh tokens.
type SessionMeta struct {
	UserAgent          string
	IP                 string
	ImpersonatorUserID *int64
	// OrganizationID is the internal organizations.id to persist on the refresh row.
	OrganizationID *int64
}

// SessionSwitch tells clients which identity the issued tokens represent.
type SessionSwitch struct {
	UserUUID         uuid.UUID  `json:"user_uuid"`
	Email            string     `json:"email"`
	ImpersonatorUUID *uuid.UUID `json:"impersonator_uuid,omitempty"`
}

// ImpersonationResult is returned when starting or stopping impersonation.
type ImpersonationResult struct {
	Tokens
	Session SessionSwitch `json:"session"`
}

// DefaultMeLinks returns fixed self-service path map.
func DefaultMeLinks() MeLinks {
	return MeLinks{
		Profile:                 "/v1/auth/profile",
		ChangePassword:          "/v1/auth/password/change",
		NotificationPreferences: "/v1/notification-preferences",
	}
}

// ToPublicUser maps a domain user to the API projection.
func ToPublicUser(u User, isSuperAdmin bool) PublicUser {
	return PublicUser{
		UUID: u.UUID, Email: u.Email, Name: u.Name, Surname: u.Surname,
		Status: u.Status, Locale: u.Locale, IsSuperAdmin: isSuperAdmin, EmailVerified: u.EmailVerified,
	}
}

// UserTOTP is the persisted authenticator binding for a user.
type UserTOTP struct {
	UserID         int64
	SecretEnc      string
	Enabled        bool
	ConfirmedAt    *time.Time
	RecoveryHashes []string
}

// TOTPStatus is the safe profile projection for 2FA.
type TOTPStatus struct {
	Enabled     bool       `json:"enabled"`
	ConfirmedAt *time.Time `json:"confirmed_at,omitempty"`
}

// TOTPSetupResult is returned when starting authenticator enrollment.
type TOTPSetupResult struct {
	Secret     string `json:"secret"`
	OTPAuthURL string `json:"otpauth_url"`
}

// TOTPConfirmResult is returned after a successful enrollment confirm.
type TOTPConfirmResult struct {
	Enabled       bool     `json:"enabled"`
	RecoveryCodes []string `json:"recovery_codes"`
}

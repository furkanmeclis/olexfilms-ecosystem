package stepup

import "time"

// Policy is the admin-configured step-up verification policy.
type Policy struct {
	TTLHours                  int  `json:"ttl_hours"`
	PasswordEnabled           bool `json:"password_enabled"`
	PasskeyEnabled            bool `json:"passkey_enabled"`
	TOTPEnabled               bool `json:"totp_enabled"`
	PasswordLoginTOTPRequired bool `json:"password_login_totp_required"`
}

// Status is the caller's current step-up grant state.
type Status struct {
	Valid     bool       `json:"valid"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Methods   []string   `json:"methods"`
}

// Grant is issued after successful step-up verification.
type Grant struct {
	Valid     bool      `json:"valid"`
	ExpiresAt time.Time `json:"expires_at"`
	Method    string    `json:"method"`
}

// PatchPolicyInput partially updates step-up policy.
type PatchPolicyInput struct {
	TTLHours                  *int  `json:"ttl_hours,omitempty"`
	PasswordEnabled           *bool `json:"password_enabled,omitempty"`
	PasskeyEnabled            *bool `json:"passkey_enabled,omitempty"`
	TOTPEnabled               *bool `json:"totp_enabled,omitempty"`
	PasswordLoginTOTPRequired *bool `json:"password_login_totp_required,omitempty"`
}

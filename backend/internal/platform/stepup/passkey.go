package stepup

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/repository"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
)

type WebAuthnConfig struct {
	RPID          string
	RPDisplayName string
	RPOrigins     []string
}

// WebAuthn wraps go-webauthn for step-up login assertions.
type WebAuthn struct {
	w *webauthn.WebAuthn
}

// NewWebAuthn creates a WebAuthn helper when RP settings are configured.
func NewWebAuthn(cfg WebAuthnConfig) (*WebAuthn, error) {
	rpID := strings.TrimSpace(cfg.RPID)
	if rpID == "" {
		return nil, errors.New("stepup: webauthn rp id is required")
	}
	display := strings.TrimSpace(cfg.RPDisplayName)
	if display == "" {
		display = "App"
	}
	origins := cfg.RPOrigins
	if len(origins) == 0 {
		return nil, errors.New("stepup: webauthn rp origins are required")
	}
	w, err := webauthn.New(&webauthn.Config{
		RPDisplayName: display,
		RPID:          rpID,
		RPOrigins:     origins,
	})
	if err != nil {
		return nil, fmt.Errorf("stepup: webauthn init: %w", err)
	}
	return &WebAuthn{w: w}, nil
}

type webAuthnUser struct {
	uuid        uuid.UUID
	name        string
	displayName string
	credentials []webauthn.Credential
	webAuthnID  []byte
}

func (u webAuthnUser) WebAuthnID() []byte {
	if len(u.webAuthnID) > 0 {
		return u.webAuthnID
	}
	return []byte(u.uuid.String())
}

func (u webAuthnUser) WebAuthnName() string {
	return u.name
}

func (u webAuthnUser) WebAuthnDisplayName() string {
	return u.displayName
}

func (u webAuthnUser) WebAuthnCredentials() []webauthn.Credential {
	return u.credentials
}

// BeginPasskey starts a WebAuthn assertion and stores session data in Redis.
func (s *Service) BeginPasskey(ctx context.Context, userUUID uuid.UUID) (any, error) {
	policy, err := s.GetPolicy(ctx)
	if err != nil {
		return nil, err
	}
	if !policy.PasskeyEnabled {
		return nil, ErrMethodDisabled
	}
	if s.webauthn == nil {
		return nil, fmt.Errorf("%w: passkey is not configured", ErrInvalidRequest)
	}

	user, creds, err := s.loadWebAuthnUser(ctx, userUUID)
	if err != nil {
		return nil, err
	}
	if len(creds) == 0 {
		return nil, ErrNoPasskeys
	}

	options, session, err := s.webauthn.w.BeginLogin(user)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(session)
	if err != nil {
		return nil, err
	}
	if err := s.store.SetChallenge(ctx, userUUID, raw); err != nil {
		return nil, err
	}
	return options, nil
}

// VerifyPasskey validates a WebAuthn assertion and issues a grant.
func (s *Service) VerifyPasskey(ctx context.Context, userUUID uuid.UUID, r *http.Request) (Grant, error) {
	policy, err := s.GetPolicy(ctx)
	if err != nil {
		return Grant{}, err
	}
	if !policy.PasskeyEnabled {
		return Grant{}, ErrMethodDisabled
	}
	if s.webauthn == nil {
		return Grant{}, fmt.Errorf("%w: passkey is not configured", ErrInvalidRequest)
	}

	raw, err := s.store.GetChallenge(ctx, userUUID)
	if err != nil {
		return Grant{}, fmt.Errorf("%w: passkey challenge expired", ErrInvalidRequest)
	}
	var session webauthn.SessionData
	if err := json.Unmarshal(raw, &session); err != nil {
		return Grant{}, err
	}

	user, _, err := s.loadWebAuthnUser(ctx, userUUID)
	if err != nil {
		return Grant{}, err
	}

	parsed, err := protocol.ParseCredentialRequestResponse(r)
	if err != nil {
		return Grant{}, ErrInvalidCredentials
	}
	// Auth.js registers passkeys with a random user handle, not the user's UUID.
	if handle := parsed.Response.UserHandle; len(handle) > 0 {
		user.webAuthnID = append([]byte(nil), handle...)
		session.UserID = append([]byte(nil), handle...)
	}
	credential, err := s.webauthn.w.ValidateLogin(user, session, parsed)
	if err != nil {
		return Grant{}, ErrInvalidCredentials
	}
	storedCredentialID, err := s.findStoredCredentialID(ctx, userUUID, credential.ID)
	if err != nil {
		return Grant{}, ErrInvalidCredentials
	}
	if err := s.repo.UpdatePasskeyCounter(ctx, storedCredentialID, int64(credential.Authenticator.SignCount)); err != nil {
		return Grant{}, err
	}
	return s.issueGrant(ctx, userUUID, "passkey", policy)
}

func (s *Service) loadWebAuthnUser(ctx context.Context, userUUID uuid.UUID) (webAuthnUser, []webauthn.Credential, error) {
	u, err := s.repo.FindUserByUUID(ctx, userUUID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return webAuthnUser{}, nil, ErrInvalidCredentials
		}
		return webAuthnUser{}, nil, err
	}
	rows, err := s.repo.ListPasskeysByUserID(ctx, u.ID)
	if err != nil {
		return webAuthnUser{}, nil, err
	}
	creds := make([]webauthn.Credential, 0, len(rows))
	for _, row := range rows {
		cred, convErr := mapPasskeyCredential(row)
		if convErr != nil {
			continue
		}
		creds = append(creds, cred)
	}
	display := strings.TrimSpace(u.Name + " " + u.Surname)
	if display == "" {
		display = u.Email
	}
	return webAuthnUser{
		uuid:        u.UUID,
		name:        u.Email,
		displayName: display,
		credentials: creds,
	}, creds, nil
}

func mapPasskeyCredential(row model.PasskeyRecord) (webauthn.Credential, error) {
	id, err := decodeBase64URL(row.CredentialID)
	if err != nil {
		return webauthn.Credential{}, err
	}
	pub, err := decodeBase64URL(row.PublicKey)
	if err != nil {
		return webauthn.Credential{}, err
	}
	return webauthn.Credential{
		ID:        id,
		PublicKey: pub,
		Transport: parseTransports(row.Transports),
		Flags: webauthn.CredentialFlags{
			UserPresent:    true,
			UserVerified:   true,
			BackupEligible: row.BackedUp,
			BackupState:    row.BackedUp,
		},
		Authenticator: webauthn.Authenticator{
			SignCount: uint32(row.Counter),
		},
	}, nil
}

func decodeBase64URL(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("empty value")
	}
	if b, err := base64.RawURLEncoding.DecodeString(raw); err == nil {
		return b, nil
	}
	if b, err := base64.URLEncoding.DecodeString(raw); err == nil {
		return b, nil
	}
	return base64.StdEncoding.DecodeString(raw)
}

func (s *Service) findStoredCredentialID(ctx context.Context, userUUID uuid.UUID, rawID []byte) (string, error) {
	u, err := s.repo.FindUserByUUID(ctx, userUUID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return "", ErrInvalidCredentials
		}
		return "", err
	}
	rows, err := s.repo.ListPasskeysByUserID(ctx, u.ID)
	if err != nil {
		return "", err
	}
	for _, row := range rows {
		id, convErr := decodeBase64URL(row.CredentialID)
		if convErr != nil {
			continue
		}
		if bytes.Equal(id, rawID) {
			return row.CredentialID, nil
		}
	}
	return "", ErrInvalidCredentials
}

func parseTransports(raw *string) []protocol.AuthenticatorTransport {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil
	}
	parts := strings.Split(*raw, ",")
	out := make([]protocol.AuthenticatorTransport, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, protocol.AuthenticatorTransport(part))
	}
	return out
}

package usecase

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/crypto"
	"github.com/jackc/pgx/v5/pgtype"
)

type githubSettingsRepo struct {
	row db.GithubAppSetting
}

func (r *githubSettingsRepo) GetGitHubAppSettings(_ context.Context) (db.GithubAppSetting, error) {
	return r.row, nil
}

func (r *githubSettingsRepo) UpdateGitHubAppSettings(_ context.Context, params db.UpdateGitHubAppSettingsParams) (db.GithubAppSetting, error) {
	if params.Enabled.Valid {
		r.row.Enabled = params.Enabled.Bool
	}
	if params.RegisterEnabled.Valid {
		r.row.RegisterEnabled = params.RegisterEnabled.Bool
	}
	if params.AppID.Valid {
		r.row.AppID = params.AppID.String
	}
	if params.ClientID.Valid {
		r.row.ClientID = params.ClientID.String
	}
	if params.ClientSecretEnc.Valid {
		r.row.ClientSecretEnc = params.ClientSecretEnc
	}
	if params.PrivateKeyEnc.Valid {
		r.row.PrivateKeyEnc = params.PrivateKeyEnc
	}
	return r.row, nil
}

func testGitHubBox(t *testing.T) *crypto.SecretBox {
	t.Helper()
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i + 11)
	}
	box, err := crypto.NewSecretBox(base64.StdEncoding.EncodeToString(raw))
	if err != nil {
		t.Fatalf("NewSecretBox: %v", err)
	}
	return box
}

func TestGitHubServiceGetMasksSecrets(t *testing.T) {
	enc, err := testGitHubBox(t).Encrypt("top-secret")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	repo := &githubSettingsRepo{row: db.GithubAppSetting{
		Enabled:         true,
		AppID:           "123",
		ClientID:        "client-id",
		ClientSecretEnc: pgtype.Text{String: enc, Valid: true},
	}}
	svc := New(repo, testGitHubBox(t))

	settings, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !settings.ClientSecretConfigured {
		t.Fatal("expected client secret configured flag")
	}
	if settings.AppID != "123" {
		t.Fatalf("unexpected app id: %q", settings.AppID)
	}
}

func TestGitHubServicePatchEnableRequiresSecret(t *testing.T) {
	repo := &githubSettingsRepo{row: db.GithubAppSetting{ClientID: "client-id"}}
	svc := New(repo, testGitHubBox(t))

	enabled := true
	_, err := svc.Patch(context.Background(), PatchInput{Enabled: &enabled})
	if err == nil {
		t.Fatal("expected validation error when enabling without secret")
	}
}

func TestGitHubServiceIsAuthEnabled(t *testing.T) {
	enc, err := testGitHubBox(t).Encrypt("secret")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	repo := &githubSettingsRepo{row: db.GithubAppSetting{
		Enabled:         true,
		ClientID:        "client-id",
		ClientSecretEnc: pgtype.Text{String: enc, Valid: true},
	}}
	svc := New(repo, testGitHubBox(t))

	ok, err := svc.IsAuthEnabled(context.Background())
	if err != nil {
		t.Fatalf("IsAuthEnabled: %v", err)
	}
	if !ok {
		t.Fatal("expected auth enabled")
	}
}

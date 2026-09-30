package usecase

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/crypto"
	"github.com/google/uuid"
)

func testOAuthBox(t *testing.T) *crypto.SecretBox {
	t.Helper()
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i + 9)
	}
	box, err := crypto.NewSecretBox(base64.StdEncoding.EncodeToString(raw))
	if err != nil {
		t.Fatalf("NewSecretBox: %v", err)
	}
	return box
}

type oauthMemRepo struct {
	memRepo
	accounts []model.OAuthAccountRecord
}

func (r *oauthMemRepo) GetOAuthAccountByProviderAccount(_ context.Context, provider, providerAccountID string) (model.OAuthAccountRecord, error) {
	for _, row := range r.accounts {
		if row.Provider == provider && row.ProviderAccountID == providerAccountID {
			return row, nil
		}
	}
	return model.OAuthAccountRecord{}, repository.ErrNotFound
}

func (r *oauthMemRepo) GetOAuthAccountByUserProvider(_ context.Context, userID int64, provider string) (model.OAuthAccountRecord, error) {
	for _, row := range r.accounts {
		if row.UserID == userID && row.Provider == provider {
			return row, nil
		}
	}
	return model.OAuthAccountRecord{}, repository.ErrNotFound
}

func (r *oauthMemRepo) ListOAuthAccountsByUserID(_ context.Context, userID int64) ([]model.OAuthAccountRecord, error) {
	out := make([]model.OAuthAccountRecord, 0)
	for _, row := range r.accounts {
		if row.UserID == userID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *oauthMemRepo) CreateOAuthAccount(_ context.Context, in model.CreateOAuthAccountInput) (model.OAuthAccountRecord, error) {
	user, err := r.FindUserByID(context.Background(), in.UserID)
	if err != nil {
		return model.OAuthAccountRecord{}, err
	}
	row := model.OAuthAccountRecord{
		UUID: uuid.New(), UserID: in.UserID, UserUUID: user.UUID,
		Provider: in.Provider, ProviderAccountID: in.ProviderAccountID, Type: in.Type,
		GitHubLogin: in.GitHubLogin,
	}
	r.accounts = append(r.accounts, row)
	return row, nil
}

func (r *oauthMemRepo) DeleteOAuthAccountByProviderAccount(_ context.Context, provider, providerAccountID string) error {
	filtered := r.accounts[:0]
	for _, row := range r.accounts {
		if row.Provider == provider && row.ProviderAccountID == providerAccountID {
			continue
		}
		filtered = append(filtered, row)
	}
	r.accounts = filtered
	return nil
}

func (r *oauthMemRepo) DeleteOAuthAccountByUserProvider(_ context.Context, userID int64, provider string) error {
	filtered := r.accounts[:0]
	found := false
	for _, row := range r.accounts {
		if row.UserID == userID && row.Provider == provider {
			found = true
			continue
		}
		filtered = append(filtered, row)
	}
	if !found {
		return repository.ErrNotFound
	}
	r.accounts = filtered
	return nil
}

func TestOAuthLinkAndListIdentities(t *testing.T) {
	userUUID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	user := model.User{ID: 1, UUID: userUUID, Email: "a@example.com", Status: "active"}
	base := &memRepo{
		users:   map[int64]model.User{1: user},
		byUUID:  map[uuid.UUID]model.User{userUUID: user},
		byEmail: map[string]model.User{"a@example.com": user},
	}
	repo := &oauthMemRepo{memRepo: *base}
	uc := NewOAuth(repo, testOAuthBox(t))

	login := "octocat"
	err := uc.LinkOAuthAccount(context.Background(), model.LinkOAuthAccountInput{
		UserID:            userUUID.String(),
		Provider:          "github",
		ProviderAccountID: "42",
		Type:              "oauth",
		GitHubLogin:       &login,
	})
	if err != nil {
		t.Fatalf("LinkOAuthAccount: %v", err)
	}

	list, err := uc.ListIdentities(context.Background(), userUUID)
	if err != nil {
		t.Fatalf("ListIdentities: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].Provider != "github" {
		t.Fatalf("unexpected identities: %+v", list.Items)
	}
}

func TestOAuthLinkConflictWhenOwnedByAnotherUser(t *testing.T) {
	userA := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	userB := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	userAModel := model.User{ID: 1, UUID: userA, Email: "a@example.com", Status: "active"}
	userBModel := model.User{ID: 2, UUID: userB, Email: "b@example.com", Status: "active"}
	base := &memRepo{
		users: map[int64]model.User{1: userAModel, 2: userBModel},
		byUUID: map[uuid.UUID]model.User{
			userA: userAModel,
			userB: userBModel,
		},
	}
	repo := &oauthMemRepo{memRepo: *base, accounts: []model.OAuthAccountRecord{
		{UserID: 1, UserUUID: userA, Provider: "github", ProviderAccountID: "42"},
	}}
	uc := NewOAuth(repo, testOAuthBox(t))

	err := uc.LinkOAuthAccount(context.Background(), model.LinkOAuthAccountInput{
		UserID:            userB.String(),
		Provider:          "github",
		ProviderAccountID: "42",
		Type:              "oauth",
	})
	if err != ErrConflict {
		t.Fatalf("expected conflict, got %v", err)
	}
}

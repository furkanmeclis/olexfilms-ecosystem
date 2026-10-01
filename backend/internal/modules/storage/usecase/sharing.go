package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/storage/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Service) Star(ctx context.Context, actor model.Actor, key string) error {
	key, err := normalizeKey(key)
	if err != nil {
		return err
	}
	if s.q == nil || actor.UserID == 0 {
		return ErrForbidden
	}
	if _, err := s.store.Head(ctx, key); err != nil {
		return ErrNotFound
	}
	_, err = s.q.InsertStorageStar(ctx, db.InsertStorageStarParams{UserID: actor.UserID, ObjectKey: key})
	if err != nil {
		return err
	}
	s.record(ctx, actor, key, "file.starred", nil)
	return nil
}

func (s *Service) Unstar(ctx context.Context, actor model.Actor, key string) error {
	key, err := normalizeKey(key)
	if err != nil {
		return err
	}
	if s.q == nil || actor.UserID == 0 {
		return ErrForbidden
	}
	return s.q.DeleteStorageStar(ctx, db.DeleteStorageStarParams{UserID: actor.UserID, ObjectKey: key})
}

func (s *Service) Shares(ctx context.Context, key string) ([]model.Share, error) {
	key, err := normalizeKey(key)
	if err != nil {
		return nil, err
	}
	if s.q == nil {
		return []model.Share{}, nil
	}
	rows, err := s.q.ListStorageSharesByKey(ctx, key)
	if err != nil {
		return nil, err
	}
	out := make([]model.Share, 0, len(rows))
	for _, row := range rows {
		out = append(out, model.Share{
			UUID:      row.Uuid.String(),
			ObjectKey: row.ObjectKey,
			UserUUID:  row.UserUuid.String(),
			Name:      strings.TrimSpace(row.UserName + " " + row.UserSurname),
			Email:     row.UserEmail.String,
			Role:      row.Role,
			CreatedAt: formatTS(row.CreatedAt),
		})
	}
	return out, nil
}

func (s *Service) Share(ctx context.Context, actor model.Actor, in model.CreateShareInput) (model.Share, error) {
	key, err := normalizeKey(in.Key)
	if err != nil {
		return model.Share{}, err
	}
	role := strings.ToLower(strings.TrimSpace(in.Role))
	if role != "viewer" && role != "editor" && role != "owner" {
		return model.Share{}, ErrInvalidKey
	}
	userUUID, err := uuid.Parse(in.UserUUID)
	if err != nil {
		return model.Share{}, ErrInvalidKey
	}
	if s.q == nil {
		return model.Share{}, ErrNotFound
	}
	user, err := s.q.GetUserByUUID(ctx, userUUID)
	if err != nil {
		return model.Share{}, ErrNotFound
	}
	row, err := s.q.InsertStorageShare(ctx, db.InsertStorageShareParams{
		ObjectKey: key,
		UserID:    user.ID,
		Role:      role,
		CreatedBy: pgInt8(actor.UserID),
	})
	if err != nil {
		return model.Share{}, err
	}
	s.record(ctx, actor, key, "file.shared", map[string]any{"user": user.Email, "role": role})
	return model.Share{
		UUID:      row.Uuid.String(),
		ObjectKey: key,
		UserUUID:  user.Uuid.String(),
		Name:      strings.TrimSpace(user.Name + " " + user.Surname),
		Email:     user.Email.String,
		Role:      role,
		CreatedAt: formatTS(row.CreatedAt),
	}, nil
}

func (s *Service) Unshare(ctx context.Context, actor model.Actor, shareUUID string) error {
	id, err := uuid.Parse(shareUUID)
	if err != nil {
		return ErrInvalidKey
	}
	if s.q == nil {
		return ErrNotFound
	}
	if err := s.q.DeleteStorageShare(ctx, id); err != nil {
		return err
	}
	_ = actor
	return nil
}

func (s *Service) Links(ctx context.Context, key string) ([]model.Link, error) {
	key, err := normalizeKey(key)
	if err != nil {
		return nil, err
	}
	if s.q == nil {
		return []model.Link{}, nil
	}
	rows, err := s.q.ListStorageLinksByKey(ctx, key)
	if err != nil {
		return nil, err
	}
	out := make([]model.Link, 0, len(rows))
	for _, row := range rows {
		out = append(out, s.linkFromRow(row, ""))
	}
	return out, nil
}

func (s *Service) CreateLink(ctx context.Context, actor model.Actor, in model.CreateLinkInput) (model.Link, error) {
	key, err := normalizeKey(in.Key)
	if err != nil {
		return model.Link{}, err
	}
	if _, err := s.store.Head(ctx, key); err != nil {
		return model.Link{}, ErrNotFound
	}
	kind := strings.ToLower(strings.TrimSpace(in.Kind))
	if kind != "public" && kind != "signed" {
		return model.Link{}, ErrInvalidKey
	}
	canView, canDownload := true, true
	if in.CanView != nil {
		canView = *in.CanView
	}
	if in.CanDownload != nil {
		canDownload = *in.CanDownload
	}
	var expires pgtype.Timestamptz
	if in.ExpiresIn > 0 {
		expires = ts(time.Now().Add(time.Duration(in.ExpiresIn) * time.Second))
	}
	slug := strings.TrimSpace(in.Slug)
	token := ""
	tokenHash := pgtype.Text{}
	if kind == "public" {
		if slug == "" {
			slug, err = randomSlug(10)
			if err != nil {
				return model.Link{}, err
			}
		}
	} else {
		token, err = randomToken(32)
		if err != nil {
			return model.Link{}, err
		}
		sum := sha256.Sum256([]byte(token))
		tokenHash = pgText(hex.EncodeToString(sum[:]))
		slug = ""
	}
	if s.q == nil {
		return model.Link{}, ErrNotFound
	}
	row, err := s.q.InsertStorageLink(ctx, db.InsertStorageLinkParams{
		ObjectKey:   key,
		Kind:        kind,
		Slug:        pgText(slug),
		TokenHash:   tokenHash,
		ExpiresAt:   expires,
		CanView:     canView,
		CanDownload: canDownload,
		CanUpload:   in.CanUpload,
		CreatedBy:   pgInt8(actor.UserID),
	})
	if err != nil {
		return model.Link{}, err
	}
	action := "link.public_created"
	if kind == "signed" {
		action = "link.signed_created"
	}
	s.record(ctx, actor, key, action, map[string]any{"kind": kind})
	return s.linkFromRow(row, token), nil
}

func (s *Service) RevokeLink(ctx context.Context, actor model.Actor, linkUUID string) error {
	id, err := uuid.Parse(linkUUID)
	if err != nil {
		return ErrInvalidKey
	}
	if s.q == nil {
		return ErrNotFound
	}
	row, err := s.q.RevokeStorageLink(ctx, id)
	if err != nil {
		if isNoRows(err) {
			return ErrNotFound
		}
		return err
	}
	s.record(ctx, actor, row.ObjectKey, "link.revoked", map[string]any{"uuid": linkUUID})
	return nil
}

func (s *Service) ResolvePublic(ctx context.Context, slug string, download bool) (model.OpenObject, error) {
	if s.q == nil {
		return model.OpenObject{}, ErrNotFound
	}
	row, err := s.q.GetStorageLinkBySlug(ctx, pgText(slug))
	if err != nil {
		return model.OpenObject{}, ErrNotFound
	}
	return s.openLink(ctx, row, download)
}

func (s *Service) ResolveSigned(ctx context.Context, token string, download bool) (model.OpenObject, error) {
	if s.q == nil || token == "" {
		return model.OpenObject{}, ErrNotFound
	}
	sum := sha256.Sum256([]byte(token))
	row, err := s.q.GetStorageLinkByTokenHash(ctx, pgText(hex.EncodeToString(sum[:])))
	if err != nil {
		return model.OpenObject{}, ErrNotFound
	}
	return s.openLink(ctx, row, download)
}

func (s *Service) openLink(ctx context.Context, row db.StorageLink, download bool) (model.OpenObject, error) {
	if row.RevokedAt.Valid {
		return model.OpenObject{}, ErrLinkRevoked
	}
	if row.ExpiresAt.Valid && time.Now().After(row.ExpiresAt.Time) {
		return model.OpenObject{}, ErrLinkExpired
	}
	if download && !row.CanDownload {
		return model.OpenObject{}, ErrForbidden
	}
	if !download && !row.CanView {
		return model.OpenObject{}, ErrForbidden
	}
	return s.Open(ctx, row.ObjectKey, "", download)
}

func (s *Service) linkFromRow(row db.StorageLink, token string) model.Link {
	status := "active"
	if row.RevokedAt.Valid {
		status = "revoked"
	} else if row.ExpiresAt.Valid && time.Now().After(row.ExpiresAt.Time) {
		status = "expired"
	}
	url := ""
	if row.Kind == "public" && row.Slug.Valid {
		url = fmt.Sprintf(publicPathFmt, row.Slug.String)
	} else if token != "" {
		url = fmt.Sprintf(signedPathFmt, token)
	}
	return model.Link{
		UUID:        row.Uuid.String(),
		ObjectKey:   row.ObjectKey,
		Kind:        row.Kind,
		Slug:        textVal(row.Slug),
		URL:         url,
		Token:       token,
		ExpiresAt:   formatTS(row.ExpiresAt),
		CanView:     row.CanView,
		CanDownload: row.CanDownload,
		CanUpload:   row.CanUpload,
		Status:      status,
		CreatedAt:   formatTS(row.CreatedAt),
	}
}

func randomSlug(n int) (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i, b := range buf {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(out), nil
}

func randomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/storage/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Service) Versions(ctx context.Context, key string) ([]model.Version, error) {
	key, err := normalizeKey(key)
	if err != nil {
		return nil, err
	}
	vers, err := s.store.ListVersions(ctx, key)
	if err != nil {
		info, headErr := s.store.Head(ctx, key)
		if headErr != nil {
			return nil, ErrNotFound
		}
		return []model.Version{{
			VersionID: info.VersionID,
			Label:     "v1",
			Size:      info.Size,
			ETag:      info.ETag,
			IsLatest:  true,
			Status:    "current",
			CreatedAt: info.LastModified.UTC().Format(time.RFC3339),
		}}, nil
	}
	if len(vers) == 0 {
		info, headErr := s.store.Head(ctx, key)
		if headErr != nil {
			return nil, ErrNotFound
		}
		return []model.Version{{
			VersionID: info.VersionID,
			Label:     "v1",
			Size:      info.Size,
			ETag:      info.ETag,
			IsLatest:  true,
			Status:    "current",
			CreatedAt: info.LastModified.UTC().Format(time.RFC3339),
		}}, nil
	}
	out := make([]model.Version, 0, len(vers))
	total := len(vers)
	for i, v := range vers {
		label := "v" + strconv.Itoa(total-i)
		status := "archived"
		if v.IsLatest {
			status = "current"
		}
		out = append(out, model.Version{
			VersionID: v.VersionID,
			Label:     label,
			Size:      v.Size,
			ETag:      v.ETag,
			IsLatest:  v.IsLatest,
			Status:    status,
			CreatedAt: v.LastModified.UTC().Format(time.RFC3339),
		})
	}
	return out, nil
}

func (s *Service) RestoreVersion(ctx context.Context, actor model.Actor, in model.RestoreVersionInput) (model.Object, error) {
	key, err := normalizeKey(in.Key)
	if err != nil {
		return model.Object{}, err
	}
	if in.VersionID == "" {
		return model.Object{}, ErrInvalidKey
	}
	if err := s.store.CopyVersion(ctx, key, key, in.VersionID); err != nil {
		return model.Object{}, fmt.Errorf("storage: restore version: %w", err)
	}
	s.record(ctx, actor, key, "version.restored", map[string]any{"version_id": in.VersionID})
	s.invalidateUsage()
	return s.Get(ctx, actor, key)
}

func (s *Service) DeleteVersion(ctx context.Context, actor model.Actor, key, versionID string) error {
	key, err := normalizeKey(key)
	if err != nil {
		return err
	}
	if versionID == "" {
		return ErrInvalidKey
	}
	if err := s.store.DeleteVersion(ctx, key, versionID); err != nil {
		return fmt.Errorf("storage: delete version: %w", err)
	}
	s.record(ctx, actor, key, "version.deleted", map[string]any{"version_id": versionID})
	return nil
}

func (s *Service) Activity(ctx context.Context, key string, limit, offset int32) ([]model.Activity, int64, error) {
	key, err := normalizeKey(key)
	if err != nil {
		return nil, 0, err
	}
	if s.q == nil {
		return []model.Activity{}, 0, nil
	}
	if limit <= 0 {
		limit = apiquery.DefaultLimit
	}
	total, err := s.q.CountStorageActivity(ctx, key)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.q.ListStorageActivity(ctx, db.ListStorageActivityParams{
		ObjectKey:   key,
		LimitCount:  limit,
		OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Activity, 0, len(rows))
	for _, row := range rows {
		actorName := strings.TrimSpace(textVal(row.ActorName) + " " + textVal(row.ActorSurname))
		if actorName == "" {
			actorName = textVal(row.ActorEmail)
		}
		if actorName == "" {
			actorName = "System"
		}
		payload := map[string]any{}
		if len(row.Payload) > 0 {
			_ = json.Unmarshal(row.Payload, &payload)
		}
		out = append(out, model.Activity{
			UUID:      row.Uuid.String(),
			ObjectKey: row.ObjectKey,
			Action:    row.Action,
			Actor:     actorName,
			Payload:   payload,
			CreatedAt: formatTS(row.CreatedAt),
		})
	}
	return out, total, nil
}

func textVal(v pgtype.Text) string {
	if !v.Valid {
		return ""
	}
	return strings.TrimSpace(v.String)
}

package usecase

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/storage/model"
	platstorage "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
)

func (s *Service) Copy(ctx context.Context, actor model.Actor, in model.CopyMoveInput) (model.Object, error) {
	src, dest, err := s.resolveCopy(in)
	if err != nil {
		return model.Object{}, err
	}
	keys, err := s.listKeysUnder(ctx, src)
	if err != nil {
		return model.Object{}, err
	}
	for _, key := range keys {
		target := remapKey(src, dest, key)
		if err := s.store.Copy(ctx, key, target); err != nil {
			return model.Object{}, fmt.Errorf("storage: copy: %w", err)
		}
	}
	s.invalidateUsage()
	s.record(ctx, actor, dest, "file.copied", map[string]any{"from": src, "to": dest})
	return s.Get(ctx, actor, dest)
}

func (s *Service) Move(ctx context.Context, actor model.Actor, in model.CopyMoveInput) (model.Object, error) {
	src, dest, err := s.resolveCopy(in)
	if err != nil {
		return model.Object{}, err
	}
	keys, err := s.listKeysUnder(ctx, src)
	if err != nil {
		return model.Object{}, err
	}
	for _, key := range keys {
		target := remapKey(src, dest, key)
		if err := s.store.Copy(ctx, key, target); err != nil {
			return model.Object{}, fmt.Errorf("storage: move copy: %w", err)
		}
	}
	for i := len(keys) - 1; i >= 0; i-- {
		if err := s.store.Delete(ctx, keys[i]); err != nil {
			return model.Object{}, fmt.Errorf("storage: move delete: %w", err)
		}
	}
	s.invalidateUsage()
	s.record(ctx, actor, dest, "file.moved", map[string]any{"from": src, "to": dest})
	return s.Get(ctx, actor, dest)
}

func (s *Service) Rename(ctx context.Context, actor model.Actor, in model.RenameInput) (model.Object, error) {
	src, err := normalizeKey(in.Key)
	if err != nil {
		return model.Object{}, err
	}
	dest, err := joinKey(parentPrefix(src), in.Name)
	if err != nil {
		return model.Object{}, err
	}
	if isFolderKey(src) {
		dest = folderKey(dest)
	}
	return s.Move(ctx, actor, model.CopyMoveInput{SourceKey: src, DestKey: dest})
}

func (s *Service) Delete(ctx context.Context, actor model.Actor, keys []string) error {
	for _, raw := range keys {
		key, err := normalizeKey(raw)
		if err != nil {
			return err
		}
		if skipSystem(key) {
			return ErrInvalidKey
		}
		children, err := s.listKeysUnder(ctx, key)
		if err != nil {
			return err
		}
		for _, child := range children {
			if err := s.trashOne(ctx, actor, child); err != nil {
				return err
			}
		}
	}
	s.invalidateUsage()
	return nil
}

func (s *Service) trashOne(ctx context.Context, actor model.Actor, key string) error {
	info, err := s.store.Head(ctx, key)
	if err != nil {
		return ErrNotFound
	}
	trashKey := platstorage.TrashPrefix + uuid.NewString() + "/" + strings.TrimPrefix(key, "/")
	if err := s.store.Copy(ctx, key, trashKey); err != nil {
		return fmt.Errorf("storage: trash copy: %w", err)
	}
	if err := s.store.Delete(ctx, key); err != nil {
		return fmt.Errorf("storage: trash delete: %w", err)
	}
	if s.q != nil {
		_, err = s.q.InsertStorageTrash(ctx, db.InsertStorageTrashParams{
			ObjectKey:   trashKey,
			OriginalKey: key,
			Name:        baseName(key),
			SizeBytes:   info.Size,
			MimeType:    mimeFromName(baseName(key), info.ContentType),
			DeletedBy:   pgInt8(actor.UserID),
			ExpiresAt:   ts(time.Now().Add(trashRetention)),
		})
		if err != nil {
			s.log.Warn("storage_trash_row_failed", "error", err)
		}
	}
	s.record(ctx, actor, key, "file.deleted", map[string]any{"name": baseName(key)})
	return nil
}

func (s *Service) Restore(ctx context.Context, actor model.Actor, keys []string) error {
	if s.q == nil {
		return ErrNotFound
	}
	for _, raw := range keys {
		row, err := s.q.GetStorageTrashByOriginalKey(ctx, raw)
		if err != nil {
			if isNoRows(err) {
				id, parseErr := uuid.Parse(raw)
				if parseErr != nil {
					return ErrNotFound
				}
				row, err = s.q.GetStorageTrashByUUID(ctx, id)
				if err != nil {
					return ErrNotFound
				}
			} else {
				return err
			}
		}
		if err := s.store.Copy(ctx, row.ObjectKey, row.OriginalKey); err != nil {
			return fmt.Errorf("storage: restore copy: %w", err)
		}
		_ = s.store.Delete(ctx, row.ObjectKey)
		_ = s.q.DeleteStorageTrashByUUID(ctx, row.Uuid)
		s.record(ctx, actor, row.OriginalKey, "file.restored", map[string]any{"name": row.Name})
	}
	s.invalidateUsage()
	return nil
}

func (s *Service) Purge(ctx context.Context, actor model.Actor, keys []string) error {
	if s.q == nil {
		return ErrNotFound
	}
	for _, raw := range keys {
		row, err := s.q.GetStorageTrashByOriginalKey(ctx, raw)
		if err != nil {
			if isNoRows(err) {
				id, parseErr := uuid.Parse(raw)
				if parseErr != nil {
					return ErrNotFound
				}
				row, err = s.q.GetStorageTrashByUUID(ctx, id)
				if err != nil {
					return ErrNotFound
				}
			} else {
				return err
			}
		}
		_ = s.store.Delete(ctx, row.ObjectKey)
		_ = s.q.DeleteStorageTrashByUUID(ctx, row.Uuid)
		s.record(ctx, actor, row.OriginalKey, "file.purged", map[string]any{"name": row.Name})
	}
	s.invalidateUsage()
	return nil
}

func (s *Service) resolveCopy(in model.CopyMoveInput) (string, string, error) {
	src, err := normalizeKey(in.SourceKey)
	if err != nil {
		return "", "", err
	}
	dest, err := normalizeKey(in.DestKey)
	if err != nil {
		return "", "", err
	}
	if skipSystem(src) || skipSystem(dest) {
		return "", "", ErrInvalidKey
	}
	if isFolderKey(src) {
		dest = folderKey(dest)
	}
	return src, dest, nil
}

func remapKey(src, dest, key string) string {
	if key == src {
		return dest
	}
	rel := strings.TrimPrefix(key, src)
	if isFolderKey(src) {
		return dest + rel
	}
	return path.Join(dest, baseName(key))
}

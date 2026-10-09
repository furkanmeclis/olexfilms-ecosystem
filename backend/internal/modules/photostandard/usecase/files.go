package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// IntakePhotoObject resolves the storage key and MIME of the active intake
// photo of a visible service angle (TEC-500, the IntakePhotoView.url path).
func (s *Service) IntakePhotoObject(ctx context.Context, c Caller, serviceID uuid.UUID, angleKey string) (string, string, error) {
	svc, err := s.visibleService(ctx, c, serviceID)
	if err != nil {
		return "", "", err
	}
	angle, err := s.q.GetPhotoAngleByKey(ctx, db.GetPhotoAngleByKeyParams{Key: strings.TrimSpace(angleKey), BrandID: svc.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("photo_standard: angle: %w", err)
	}
	photos, err := s.q.ListActiveIntakePhotosForService(ctx, svc.ID)
	if err != nil {
		return "", "", fmt.Errorf("photo_standard: photos: %w", err)
	}
	for _, p := range photos {
		if p.AngleID == angle.ID {
			return p.StorageKey, p.Mime, nil
		}
	}
	return "", "", ErrNotFound
}

// ExampleObject resolves the storage key of an angle's example image in the
// caller's brand (TEC-500).
func (s *Service) ExampleObject(ctx context.Context, c Caller, angleID uuid.UUID) (string, error) {
	row, err := s.q.GetPhotoAngleByUUID(ctx, db.GetPhotoAngleByUUIDParams{Uuid: angleID, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !row.ExampleStorageKey.Valid) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("photo_standard: angle: %w", err)
	}
	// The key is writable through PUT; only an uploaded example is served.
	key := row.ExampleStorageKey.String
	if !strings.HasPrefix(key, "photo-standard/angles/"+row.Uuid.String()+"/") {
		return "", ErrNotFound
	}
	return key, nil
}

// ExampleSlot is the storage key a new example image is written to and the
// replaced one to delete after the upload.
type ExampleSlot struct {
	ObjectKey string
	OldKey    string
	Angle     AngleView
}

// SetExample points an angle at a new example image (TEC-500). Only raster
// formats a browser can show are accepted (no HEIC).
func (s *Service) SetExample(ctx context.Context, c Caller, angleID uuid.UUID, meta UploadMeta) (ExampleSlot, error) {
	if meta.Mime == "image/heic" {
		return ExampleSlot{}, ErrUnsupportedMedia
	}
	row, err := s.q.GetPhotoAngleByUUID(ctx, db.GetPhotoAngleByUUIDParams{Uuid: angleID, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ExampleSlot{}, ErrNotFound
	}
	if err != nil {
		return ExampleSlot{}, fmt.Errorf("photo_standard: angle: %w", err)
	}
	out := ExampleSlot{ObjectKey: storage.PhotoAngleExampleObjectKey(row.Uuid, uuid.New(), meta.Ext)}
	if row.ExampleStorageKey.Valid {
		out.OldKey = row.ExampleStorageKey.String
	}
	row, err = s.q.UpdatePhotoAngle(ctx, db.UpdatePhotoAngleParams{
		ID: row.ID, BrandID: c.Org.BrandID, Name: row.Name, Hint: row.Hint,
		ExampleStorageKey: pgtype.Text{String: out.ObjectKey, Valid: true}, Required: row.Required,
		SortOrder: row.SortOrder, Active: row.Active,
	})
	if err != nil {
		return ExampleSlot{}, fmt.Errorf("photo_standard: set example: %w", err)
	}
	out.Angle = angleView(row, row.Required, false)
	return out, nil
}

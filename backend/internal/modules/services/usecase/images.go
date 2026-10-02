package usecase

import (
	"context"
	"fmt"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ImageSlot is where a new image of a service is stored.
type ImageSlot struct {
	ServiceUUID uuid.UUID
	ImageUUID   uuid.UUID
	ObjectKey   string
}

// PrepareImage checks that the caller may add an image to the service (write
// scope, editable form, below MaxImages) and returns its object key. The
// handler uploads the bytes and then calls AddImage.
func (s *Service) PrepareImage(ctx context.Context, c Caller, id uuid.UUID, ext string) (ImageSlot, error) {
	var slot ImageSlot
	err := s.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		svc, err := s.lockWritable(ctx, q, c, id)
		if err != nil {
			return err
		}
		if err := checkImageCount(ctx, q, svc.ID); err != nil {
			return err
		}
		org, err := q.GetOrganizationByID(ctx, svc.OrganizationID)
		if err != nil {
			return fmt.Errorf("services: organization: %w", err)
		}
		img := uuid.New()
		slot = ImageSlot{ServiceUUID: svc.Uuid, ImageUUID: img,
			ObjectKey: storage.ServiceImageObjectKey(org.Uuid, svc.Uuid, img, ext)}
		return nil
	})
	return slot, err
}

func checkImageCount(ctx context.Context, q *db.Queries, serviceID int64) error {
	imgs, err := q.ListServiceImages(ctx, serviceID)
	if err != nil {
		return fmt.Errorf("services: images: %w", err)
	}
	if len(imgs) >= MaxImages {
		return ErrTooManyImages
	}
	return nil
}

// AddImage records an uploaded image (the checks of PrepareImage run again
// under the row lock).
func (s *Service) AddImage(ctx context.Context, c Caller, id uuid.UUID, objectKey string, title *string) (ImageView, error) {
	if err := checkLen("title", title, 255); err != nil {
		return ImageView{}, err
	}
	var out ImageView
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		svc, err := s.lockWritable(ctx, q, c, id)
		if err != nil {
			return err
		}
		imgs, err := q.ListServiceImages(ctx, svc.ID)
		if err != nil {
			return fmt.Errorf("services: images: %w", err)
		}
		if len(imgs) >= MaxImages {
			return ErrTooManyImages
		}
		var sort int32
		for _, im := range imgs {
			if im.SortOrder >= sort {
				sort = im.SortOrder + 1
			}
		}
		im, err := q.CreateServiceImage(ctx, db.CreateServiceImageParams{
			ServiceID: svc.ID, StorageKey: objectKey, Title: textOrNull(title), SortOrder: sort,
			UploadedByUserID: c.actor(),
		})
		if err != nil {
			return fmt.Errorf("services: create image: %w", err)
		}
		out = ImageView{UUID: im.Uuid, Title: textPtr(im.Title), SortOrder: im.SortOrder,
			URL: ImageURL(svc.Uuid, im.Uuid), CreatedAt: im.CreatedAt.Time}
		return s.emit(ctx, tx, events.ServiceImageAdded, svc, "", c, map[string]any{"image_uuid": im.Uuid.String()})
	})
	return out, err
}

// RemoveImage deletes an image row and returns its object key for the
// handler to delete from storage.
func (s *Service) RemoveImage(ctx context.Context, c Caller, id, imageID uuid.UUID) (string, error) {
	var key string
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		svc, err := s.lockWritable(ctx, q, c, id)
		if err != nil {
			return err
		}
		im, err := findImage(ctx, q, svc.ID, imageID)
		if err != nil {
			return err
		}
		if _, err := q.DeleteServiceImage(ctx, db.DeleteServiceImageParams{ID: im.ID, ServiceID: svc.ID}); err != nil {
			return fmt.Errorf("services: delete image: %w", err)
		}
		key = im.StorageKey
		return s.emit(ctx, tx, events.ServiceImageRemoved, svc, "", c, map[string]any{"image_uuid": im.Uuid.String()})
	})
	return key, err
}

// ImageObject returns the object key of a visible service's image.
func (s *Service) ImageObject(ctx context.Context, c Caller, id, imageID uuid.UUID) (string, error) {
	svc, err := s.getVisible(ctx, c, id)
	if err != nil {
		return "", err
	}
	im, err := findImage(ctx, s.q, svc.ID, imageID)
	if err != nil {
		return "", err
	}
	return im.StorageKey, nil
}

func findImage(ctx context.Context, q *db.Queries, serviceID int64, imageID uuid.UUID) (db.ServiceImage, error) {
	imgs, err := q.ListServiceImages(ctx, serviceID)
	if err != nil {
		return db.ServiceImage{}, fmt.Errorf("services: images: %w", err)
	}
	for _, im := range imgs {
		if im.Uuid == imageID {
			return im, nil
		}
	}
	return db.ServiceImage{}, ErrNotFound
}

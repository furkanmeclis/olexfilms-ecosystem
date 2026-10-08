package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/dealershowcase/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/dealershowcase/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrDuplicateService: the product category is already a service of the
// showcase (409).
var ErrDuplicateService = errors.New("dealershowcase: category already listed")

// MaxPhotoUploadBytes caps one gallery upload (5 MiB; the DB allows 20).
const MaxPhotoUploadBytes = 5 << 20

// ServiceInput creates or replaces a showcase service. A product_category
// service names a category of the brand (its title is optional, the
// category name is the fallback); a custom one needs a title.
type ServiceInput struct {
	Kind         string            `json:"kind"`
	CategoryUUID *uuid.UUID        `json:"category_uuid"`
	Title        map[string]string `json:"title"`
	Description  map[string]string `json:"description"`
	Visible      *bool             `json:"visible"`
}

type serviceFields struct {
	kind        string
	categoryID  pgtype.Int8
	title, desc []byte
	visible     bool
}

func (in ServiceInput) fields(ctx context.Context, q *db.Queries, brandID int64) (serviceFields, error) {
	var f serviceFields
	if !slices.Contains(model.ServiceKinds, in.Kind) {
		return f, invalid("kind", "must be product_category or custom")
	}
	f.kind = in.Kind
	var (
		title map[string]string
		err   error
	)
	if f.title, title, err = localeTexts("title", in.Title, MaxServiceTitle); err != nil {
		return f, err
	}
	if f.desc, _, err = localeTexts("description", in.Description, MaxServiceDesc); err != nil {
		return f, err
	}
	switch in.Kind {
	case model.ServiceKindProductCategory:
		if in.CategoryUUID == nil {
			return f, invalid("category_uuid", "is required for a product_category service")
		}
		cat, err := q.GetProductCategoryByUUID(ctx, db.GetProductCategoryByUUIDParams{Uuid: *in.CategoryUUID, BrandID: brandID})
		if errors.Is(err, pgx.ErrNoRows) {
			return f, invalid("category_uuid", "unknown category")
		}
		if err != nil {
			return f, err
		}
		f.categoryID = pgtype.Int8{Int64: cat.ID, Valid: true}
	case model.ServiceKindCustom:
		if in.CategoryUUID != nil {
			return f, invalid("category_uuid", "must be empty for a custom service")
		}
		if len(title) == 0 {
			return f, invalid("title", "is required for a custom service")
		}
	}
	f.visible = in.Visible == nil || *in.Visible
	return f, nil
}

func isUnique(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

// CreateService appends a service to the showcase (opening the showcase
// when the organization has none).
func (s *Service) CreateService(ctx context.Context, c Caller, orgUUID *uuid.UUID, in ServiceInput) (ServiceItem, error) {
	var out ServiceItem
	err := s.inTx(ctx, func(_ pgx.Tx, q *db.Queries) error {
		o, err := s.target(ctx, q, c, orgUUID)
		if err != nil {
			return err
		}
		f, err := in.fields(ctx, q, o.BrandID)
		if err != nil {
			return err
		}
		row, err := ensure(ctx, q, c, o)
		if err != nil {
			return err
		}
		sv, err := q.InsertDealerShowcaseService(ctx, db.InsertDealerShowcaseServiceParams{
			ShowcaseID: row.ID, Kind: f.kind, CategoryID: f.categoryID, Title: f.title, Description: f.desc, Visible: f.visible,
		})
		if isUnique(err) {
			return ErrDuplicateService
		}
		if err != nil {
			return err
		}
		out, err = serviceItem(ctx, q, sv)
		return err
	})
	return out, err
}

// UpdateService replaces a service.
func (s *Service) UpdateService(ctx context.Context, c Caller, orgUUID *uuid.UUID, id uuid.UUID, in ServiceInput) (ServiceItem, error) {
	o, row, err := s.existing(ctx, s.q, c, orgUUID)
	if err != nil {
		return ServiceItem{}, err
	}
	f, err := in.fields(ctx, s.q, o.BrandID)
	if err != nil {
		return ServiceItem{}, err
	}
	sv, err := s.q.UpdateDealerShowcaseService(ctx, db.UpdateDealerShowcaseServiceParams{
		Uuid: id, ShowcaseID: row.ID, Kind: f.kind, CategoryID: f.categoryID, Title: f.title, Description: f.desc, Visible: f.visible,
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ServiceItem{}, ErrNotFound
	case isUnique(err):
		return ServiceItem{}, ErrDuplicateService
	case err != nil:
		return ServiceItem{}, err
	}
	return serviceItem(ctx, s.q, sv)
}

// ListServices returns the services in their order.
func (s *Service) ListServices(ctx context.Context, c Caller, orgUUID *uuid.UUID) ([]ServiceItem, error) {
	o, err := s.target(ctx, s.q, c, orgUUID)
	if err != nil {
		return nil, err
	}
	row, ok, err := showcaseOf(ctx, s.q, o.ID)
	if err != nil || !ok {
		return []ServiceItem{}, err
	}
	rows, err := s.q.ListDealerShowcaseServices(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	out := make([]ServiceItem, 0, len(rows))
	for _, sv := range rows {
		item, err := serviceItem(ctx, s.q, sv)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

// DeleteService removes a service.
func (s *Service) DeleteService(ctx context.Context, c Caller, orgUUID *uuid.UUID, id uuid.UUID) error {
	_, row, err := s.existing(ctx, s.q, c, orgUUID)
	if err != nil {
		return err
	}
	n, err := s.q.DeleteDealerShowcaseService(ctx, db.DeleteDealerShowcaseServiceParams{Uuid: id, ShowcaseID: row.ID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ReorderServices sets the service order; uuids lists every service once.
func (s *Service) ReorderServices(ctx context.Context, c Caller, orgUUID *uuid.UUID, uuids []uuid.UUID) ([]ServiceItem, error) {
	err := s.inTx(ctx, func(_ pgx.Tx, q *db.Queries) error {
		_, row, err := s.existing(ctx, q, c, orgUUID)
		if err != nil {
			return err
		}
		if _, err := q.LockDealerShowcase(ctx, row.ID); err != nil {
			return err
		}
		return mapRepoErr(repository.FromQueries(q).ReorderServices(ctx, row.ID, uuids))
	})
	if err != nil {
		return nil, err
	}
	return s.ListServices(ctx, c, orgUUID)
}

// existing resolves the target and its showcase; no showcase is
// ErrNotFound.
func (s *Service) existing(ctx context.Context, q *db.Queries, c Caller, orgUUID *uuid.UUID) (db.Organization, db.DealerShowcase, error) {
	o, err := s.target(ctx, q, c, orgUUID)
	if err != nil {
		return db.Organization{}, db.DealerShowcase{}, err
	}
	row, ok, err := showcaseOf(ctx, q, o.ID)
	if err != nil {
		return db.Organization{}, db.DealerShowcase{}, err
	}
	if !ok {
		return db.Organization{}, db.DealerShowcase{}, ErrNotFound
	}
	return o, row, nil
}

// PhotoSlot is where the handler uploads a new gallery photo.
type PhotoSlot struct {
	UUID      uuid.UUID
	ObjectKey string
}

// PreparePhoto checks the target and the gallery cap before the upload
// (a full gallery is a *repository.PhotoLimitError, 422) and returns the
// object key of the new photo: showcases/{org}/{photo}.{ext}.
func (s *Service) PreparePhoto(ctx context.Context, c Caller, orgUUID *uuid.UUID, ext string) (PhotoSlot, error) {
	o, err := s.target(ctx, s.q, c, orgUUID)
	if err != nil {
		return PhotoSlot{}, err
	}
	if row, ok, err := showcaseOf(ctx, s.q, o.ID); err != nil {
		return PhotoSlot{}, err
	} else if ok {
		n, err := s.q.CountDealerShowcasePhotos(ctx, row.ID)
		if err != nil {
			return PhotoSlot{}, err
		}
		if limit := s.maxPhotos(ctx); n >= limit {
			return PhotoSlot{}, &repository.PhotoLimitError{Count: n, Max: limit}
		}
	}
	id := uuid.New()
	return PhotoSlot{UUID: id, ObjectKey: storage.ShowcasePhotoObjectKey(o.Uuid, id, ext)}, nil
}

// PhotoInput is an uploaded photo (the handler sniffed the type).
type PhotoInput struct {
	ObjectKey string
	Mime      string
	SizeBytes int64
	SHA256    string
	Caption   map[string]string
}

// AddPhoto records an uploaded photo at the end of the gallery. The
// showcase row is locked, so concurrent uploads respect the cap. The same
// image twice is ErrDuplicatePhoto.
func (s *Service) AddPhoto(ctx context.Context, c Caller, orgUUID *uuid.UUID, in PhotoInput) (Photo, error) {
	if !slices.Contains(model.PhotoMimes, in.Mime) {
		return Photo{}, invalid("photo", "must be image/jpeg, image/png or image/webp")
	}
	if in.SizeBytes <= 0 || in.SizeBytes > MaxPhotoUploadBytes {
		return Photo{}, invalid("photo", "must be at most 5 MiB")
	}
	caption, _, err := localeTexts("caption", in.Caption, MaxCaption)
	if err != nil {
		return Photo{}, err
	}
	var out Photo
	err = s.inTx(ctx, func(_ pgx.Tx, q *db.Queries) error {
		o, err := s.target(ctx, q, c, orgUUID)
		if err != nil {
			return err
		}
		row, err := ensure(ctx, q, c, o)
		if err != nil {
			return err
		}
		p, err := repository.FromQueries(q).AddPhoto(ctx, s.maxPhotos(ctx), db.InsertDealerShowcasePhotoParams{
			ShowcaseID: row.ID, StorageKey: in.ObjectKey, Mime: in.Mime, SizeBytes: in.SizeBytes,
			Sha256: in.SHA256, Caption: caption, CreatedByUserID: c.actor(),
		})
		if isUnique(err) {
			return ErrDuplicatePhoto
		}
		if err != nil {
			return err
		}
		out = photoView(p)
		return nil
	})
	return out, err
}

// SetPhotoCaption replaces a photo's caption.
func (s *Service) SetPhotoCaption(ctx context.Context, c Caller, orgUUID *uuid.UUID, id uuid.UUID, caption map[string]string) (Photo, error) {
	raw, _, err := localeTexts("caption", caption, MaxCaption)
	if err != nil {
		return Photo{}, err
	}
	_, row, err := s.existing(ctx, s.q, c, orgUUID)
	if err != nil {
		return Photo{}, err
	}
	p, err := s.q.UpdateDealerShowcasePhotoCaption(ctx, db.UpdateDealerShowcasePhotoCaptionParams{Uuid: id, ShowcaseID: row.ID, Caption: raw})
	if errors.Is(err, pgx.ErrNoRows) {
		return Photo{}, ErrNotFound
	}
	if err != nil {
		return Photo{}, err
	}
	return photoView(p), nil
}

// DeletePhoto removes a gallery photo. Its object is deleted unless the
// live snapshot still shows it (then the next publish removes it).
func (s *Service) DeletePhoto(ctx context.Context, c Caller, orgUUID *uuid.UUID, id uuid.UUID) error {
	var drop []string
	err := s.inTx(ctx, func(_ pgx.Tx, q *db.Queries) error {
		_, row, err := s.existing(ctx, q, c, orgUUID)
		if err != nil {
			return err
		}
		if row, err = q.LockDealerShowcase(ctx, row.ID); err != nil {
			return err
		}
		key, err := q.DeleteDealerShowcasePhoto(ctx, db.DeleteDealerShowcasePhotoParams{Uuid: id, ShowcaseID: row.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !snapshotHasKey(row.PublishedContent, key) {
			drop = append(drop, key)
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.dropObjects(ctx, drop)
	return nil
}

// ReorderPhotos sets the gallery order; uuids lists every photo once.
func (s *Service) ReorderPhotos(ctx context.Context, c Caller, orgUUID *uuid.UUID, uuids []uuid.UUID) ([]Photo, error) {
	var out []Photo
	err := s.inTx(ctx, func(_ pgx.Tx, q *db.Queries) error {
		_, row, err := s.existing(ctx, q, c, orgUUID)
		if err != nil {
			return err
		}
		if _, err := q.LockDealerShowcase(ctx, row.ID); err != nil {
			return err
		}
		if err := mapRepoErr(repository.FromQueries(q).ReorderPhotos(ctx, row.ID, uuids)); err != nil {
			return err
		}
		rows, err := q.ListDealerShowcasePhotos(ctx, row.ID)
		if err != nil {
			return err
		}
		out = make([]Photo, 0, len(rows))
		for _, p := range rows {
			out = append(out, photoView(p))
		}
		return nil
	})
	return out, err
}

// PhotoObject returns the storage key and type of a gallery photo (panel
// preview).
func (s *Service) PhotoObject(ctx context.Context, c Caller, orgUUID *uuid.UUID, id uuid.UUID) (string, string, error) {
	_, row, err := s.existing(ctx, s.q, c, orgUUID)
	if err != nil {
		return "", "", err
	}
	p, err := s.q.GetDealerShowcasePhoto(ctx, db.GetDealerShowcasePhotoParams{Uuid: id, ShowcaseID: row.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("dealershowcase: photo: %w", err)
	}
	return p.StorageKey, p.Mime, nil
}

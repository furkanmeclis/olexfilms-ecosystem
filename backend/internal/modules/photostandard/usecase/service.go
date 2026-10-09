package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/photostandard/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	MaxUploadBytes = 12 << 20

	CodeUnsupportedMedia = "PHOTO_UNSUPPORTED_MEDIA"
	CodeFileTooLarge     = "PHOTO_FILE_TOO_LARGE"
	CodeServiceLocked    = "PHOTO_SERVICE_LOCKED"
)

var (
	ErrNotFound         = errors.New("photo_standard: not found")
	ErrForbidden        = errors.New("photo_standard: forbidden")
	ErrValidation       = errors.New("photo_standard: validation")
	ErrUnsupportedMedia = errors.New("photo_standard: unsupported media")
	ErrFileTooLarge     = errors.New("photo_standard: file too large")
	ErrServiceLocked    = errors.New("photo_standard: service locked")
)

type Caller struct {
	Principal authctx.Principal
	Org       orgctx.Scope
	Filter    scopefilter.Filter
}

type Service struct {
	pool     *pgxpool.Pool
	q        *db.Queries
	features FeatureChecker
	out      outbox.Enqueuer
	store    storage.Driver
}

func New(pool *pgxpool.Pool, q *db.Queries) *Service {
	return &Service{pool: pool, q: q}
}

type AngleInput struct {
	Key               string
	Name              json.RawMessage
	Hint              json.RawMessage
	ExampleStorageKey *string
	Required          bool
	SortOrder         int32
	Active            bool
}

type AngleView struct {
	UUID              uuid.UUID       `json:"uuid"`
	Key               string          `json:"key"`
	Name              json.RawMessage `json:"name"`
	Hint              json.RawMessage `json:"hint"`
	ExampleStorageKey *string         `json:"example_storage_key,omitempty"`
	// ExampleURL is the authenticated path of the example image (TEC-500).
	ExampleURL string `json:"example_url,omitempty"`
	Required   bool   `json:"required"`
	Hidden     bool   `json:"hidden,omitempty"`
	SortOrder  int32  `json:"sort_order"`
	Active     bool   `json:"active"`
	// Override form only (TEC-500): the central default and whether the
	// target organization has its own override row.
	DefaultRequired *bool `json:"default_required,omitempty"`
	DefaultHidden   *bool `json:"default_hidden,omitempty"`
	Overridden      *bool `json:"overridden,omitempty"`
}

type OverrideInput struct {
	AngleKey string
	Required bool
	Hidden   bool
}

type IntakePhotoView struct {
	UUID        uuid.UUID  `json:"uuid"`
	AngleKey    string     `json:"angle_key"`
	URL         string     `json:"url"`
	Mime        string     `json:"mime"`
	Size        int64      `json:"size"`
	SHA256      string     `json:"sha256"`
	Width       *int32     `json:"width,omitempty"`
	Height      *int32     `json:"height,omitempty"`
	ExifTakenAt *time.Time `json:"exif_taken_at,omitempty"`
	ExifLat     *string    `json:"exif_lat,omitempty"`
	ExifLng     *string    `json:"exif_lng,omitempty"`
	ExifDevice  *string    `json:"exif_device,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type IntakeAngleView struct {
	Angle    AngleView        `json:"angle"`
	Photo    *IntakePhotoView `json:"photo,omitempty"`
	Missing  bool             `json:"missing"`
	Required bool             `json:"required"`
}

type IntakeListView struct {
	ServiceUUID uuid.UUID         `json:"service_uuid"`
	Angles      []IntakeAngleView `json:"angles"`
	Missing     []string          `json:"missing"`
}

type UploadMeta struct {
	Mime        string
	Size        int64
	SHA256      string
	Width       *int32
	Height      *int32
	ExifTakenAt *time.Time
	ExifLat     *string
	ExifLng     *string
	ExifDevice  *string
	Ext         string
}

type UploadSlot struct {
	ObjectKey string
	OldKey    string
	Photo     IntakePhotoView
}

func (s *Service) CreateAngle(ctx context.Context, c Caller, in AngleInput) (AngleView, error) {
	if err := validateAngle(in); err != nil {
		return AngleView{}, err
	}
	row, err := s.q.CreatePhotoAngle(ctx, db.CreatePhotoAngleParams{
		OrganizationID:    c.Org.InternalID,
		BrandID:           c.Org.BrandID,
		Key:               strings.TrimSpace(in.Key),
		Name:              []byte(in.Name),
		Hint:              hintOrEmpty(in.Hint),
		ExampleStorageKey: textOrNull(in.ExampleStorageKey),
		Required:          in.Required,
		SortOrder:         in.SortOrder,
		Active:            in.Active,
	})
	if err != nil {
		return AngleView{}, fmt.Errorf("photo_standard: create angle: %w", err)
	}
	return angleView(row, row.Required, false), nil
}

func (s *Service) ListAngles(ctx context.Context, c Caller) ([]AngleView, error) {
	rows, err := s.q.ListPhotoAnglesByBrand(ctx, c.Org.BrandID)
	if err != nil {
		return nil, fmt.Errorf("photo_standard: list angles: %w", err)
	}
	out := make([]AngleView, 0, len(rows))
	for _, row := range rows {
		out = append(out, angleView(row, row.Required, false))
	}
	return out, nil
}

func (s *Service) UpdateAngle(ctx context.Context, c Caller, id uuid.UUID, in AngleInput) (AngleView, error) {
	if err := validateAngle(in); err != nil {
		return AngleView{}, err
	}
	row, err := s.q.GetPhotoAngleByUUID(ctx, db.GetPhotoAngleByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return AngleView{}, ErrNotFound
	}
	if err != nil {
		return AngleView{}, fmt.Errorf("photo_standard: angle: %w", err)
	}
	row, err = s.q.UpdatePhotoAngle(ctx, db.UpdatePhotoAngleParams{
		ID: row.ID, BrandID: c.Org.BrandID, Name: []byte(in.Name), Hint: hintOrEmpty(in.Hint),
		ExampleStorageKey: textOrNull(in.ExampleStorageKey), Required: in.Required,
		SortOrder: in.SortOrder, Active: in.Active,
	})
	if err != nil {
		return AngleView{}, fmt.Errorf("photo_standard: update angle: %w", err)
	}
	return angleView(row, row.Required, false), nil
}

func (s *Service) DeleteAngle(ctx context.Context, c Caller, id uuid.UUID) error {
	row, err := s.q.GetPhotoAngleByUUID(ctx, db.GetPhotoAngleByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("photo_standard: angle: %w", err)
	}
	n, err := s.q.DeletePhotoAngle(ctx, db.DeletePhotoAngleParams{ID: row.ID, BrandID: c.Org.BrandID})
	if err != nil {
		return fmt.Errorf("photo_standard: delete angle: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) PutOverrides(ctx context.Context, c Caller, target uuid.UUID, items []OverrideInput) ([]AngleView, error) {
	org, err := s.q.GetOrganizationByUUID(ctx, target)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("photo_standard: target org: %w", err)
	}
	if org.BrandID != c.Org.BrandID || !c.Filter.AllowsOrg(org.ID, org.BrandID) {
		return nil, ErrForbidden
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("photo_standard: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	if err := q.DeletePhotoAngleOverridesForOrg(ctx, db.DeletePhotoAngleOverridesForOrgParams{
		OrganizationID: org.ID, BrandID: c.Org.BrandID,
	}); err != nil {
		return nil, fmt.Errorf("photo_standard: clear overrides: %w", err)
	}
	for _, item := range items {
		if item.Hidden && item.Required {
			return nil, ErrValidation
		}
		angle, err := q.GetPhotoAngleByKey(ctx, db.GetPhotoAngleByKeyParams{Key: strings.TrimSpace(item.AngleKey), BrandID: c.Org.BrandID})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, fmt.Errorf("photo_standard: angle: %w", err)
		}
		if _, err := q.UpsertPhotoAngleOverride(ctx, db.UpsertPhotoAngleOverrideParams{
			OrganizationID: org.ID, BrandID: c.Org.BrandID, AngleID: angle.ID,
			Required: item.Required, Hidden: item.Hidden, CreatedByUserID: int8OrNull(c.Principal.UserInternal),
		}); err != nil {
			return nil, fmt.Errorf("photo_standard: override: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("photo_standard: commit: %w", err)
	}
	return s.OverrideAngles(ctx, org.ID, org.BrandID)
}

func (s *Service) GetOverrides(ctx context.Context, c Caller, target uuid.UUID) ([]AngleView, error) {
	org, err := s.q.GetOrganizationByUUID(ctx, target)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("photo_standard: target org: %w", err)
	}
	if org.BrandID != c.Org.BrandID || !c.Filter.AllowsOrg(org.ID, org.BrandID) {
		return nil, ErrForbidden
	}
	return s.OverrideAngles(ctx, org.ID, org.BrandID)
}

// OverrideAngles is the override form view of an organization (TEC-500):
// every active angle, hidden ones included, with the resolved values, the
// central default and whether the organization overrides it.
func (s *Service) OverrideAngles(ctx context.Context, orgID, brandID int64) ([]AngleView, error) {
	rows, err := s.allResolvedRows(ctx, s.q, orgID, brandID)
	if err != nil {
		return nil, err
	}
	out := make([]AngleView, 0, len(rows))
	for _, row := range rows {
		v := resolvedView(row)
		v.DefaultRequired, v.DefaultHidden, v.Overridden = &row.DefaultRequired, &row.DefaultHidden, &row.Overridden
		out = append(out, v)
	}
	return out, nil
}

func (s *Service) ResolvedAngles(ctx context.Context, serviceOrgID, brandID int64) ([]AngleView, error) {
	rows, err := s.resolvedRows(ctx, s.q, serviceOrgID, brandID)
	if err != nil {
		return nil, err
	}
	out := make([]AngleView, 0, len(rows))
	for _, row := range rows {
		out = append(out, resolvedView(row))
	}
	return out, nil
}

func resolvedView(row db.ListResolvedPhotoAnglesRow) AngleView {
	id := textUUID(row.Uuid)
	return AngleView{
		UUID: id, Key: row.Key, Name: copyJSON(row.Name), Hint: copyJSON(row.Hint),
		ExampleStorageKey: textPtr(row.ExampleStorageKey), ExampleURL: exampleURL(id, row.ExampleStorageKey),
		Required: row.ResolvedRequired, Hidden: row.ResolvedHidden, SortOrder: row.SortOrder, Active: row.Active,
	}
}

func (s *Service) Intake(ctx context.Context, c Caller, serviceID uuid.UUID) (IntakeListView, error) {
	svc, err := s.visibleService(ctx, c, serviceID)
	if err != nil {
		return IntakeListView{}, err
	}
	angles, err := s.ResolvedAngles(ctx, svc.OrganizationID, svc.BrandID)
	if err != nil {
		return IntakeListView{}, err
	}
	photos, err := s.q.ListActiveIntakePhotosForService(ctx, svc.ID)
	if err != nil {
		return IntakeListView{}, fmt.Errorf("photo_standard: photos: %w", err)
	}
	byAngle := map[int64]db.IntakePhoto{}
	for _, p := range photos {
		byAngle[p.AngleID] = p
	}
	out := IntakeListView{ServiceUUID: svc.Uuid, Angles: make([]IntakeAngleView, 0, len(angles))}
	exif := CanSeeEXIF(c, svc.OrganizationID)
	for _, a := range angles {
		row, err := s.q.GetPhotoAngleByKey(ctx, db.GetPhotoAngleByKeyParams{Key: a.Key, BrandID: svc.BrandID})
		if err != nil {
			return IntakeListView{}, fmt.Errorf("photo_standard: angle key: %w", err)
		}
		var photo *IntakePhotoView
		if p, ok := byAngle[row.ID]; ok {
			v := photoView(p, svc.Uuid, a.Key)
			if !exif {
				redactEXIF(&v)
			}
			photo = &v
		}
		missing := a.Required && photo == nil
		if missing {
			out.Missing = append(out.Missing, a.Key)
		}
		out.Angles = append(out.Angles, IntakeAngleView{Angle: a, Photo: photo, Missing: missing, Required: a.Required})
	}
	return out, nil
}

func (s *Service) Upload(ctx context.Context, c Caller, serviceID uuid.UUID, angleKey string, meta UploadMeta) (UploadSlot, error) {
	var out UploadSlot
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		svc, err := s.lockWritableService(ctx, q, c, serviceID)
		if err != nil {
			return err
		}
		angle, err := q.GetPhotoAngleByKey(ctx, db.GetPhotoAngleByKeyParams{Key: strings.TrimSpace(angleKey), BrandID: svc.BrandID})
		if errors.Is(err, pgx.ErrNoRows) || !angle.Active {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("photo_standard: angle: %w", err)
		}
		var missingBefore []string
		if s.out != nil {
			if missingBefore, err = s.missingAngles(ctx, q, model.ServiceRef{
				ID: svc.ID, OrganizationID: svc.OrganizationID, BrandID: svc.BrandID,
			}); err != nil {
				return err
			}
		}
		old, err := q.SoftDeleteActiveIntakePhoto(ctx, db.SoftDeleteActiveIntakePhotoParams{
			ServiceID: svc.ID, AngleID: angle.ID, DeletedBy: int8OrNull(c.Principal.UserInternal),
		})
		if err == nil {
			out.OldKey = old.StorageKey
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("photo_standard: replace old photo: %w", err)
		}
		photoID := uuid.New()
		out.ObjectKey = storage.IntakePhotoObjectKey(c.Org.UUID, svc.Uuid, angle.Key, photoID, meta.Ext)
		row, err := q.CreateIntakePhoto(ctx, db.CreateIntakePhotoParams{
			ServiceID: svc.ID, AngleID: angle.ID, StorageKey: out.ObjectKey, Mime: meta.Mime,
			Size: meta.Size, Sha256: meta.SHA256, Width: int4OrNull(meta.Width), Height: int4OrNull(meta.Height),
			ExifTakenAt: timeOrNull(meta.ExifTakenAt), ExifLat: numericOrNull(meta.ExifLat),
			ExifLng: numericOrNull(meta.ExifLng), ExifDevice: textOrNull(meta.ExifDevice),
			UploadedBy: int8OrNull(c.Principal.UserInternal),
		})
		if err != nil {
			return fmt.Errorf("photo_standard: create photo: %w", err)
		}
		out.Photo = photoView(row, svc.Uuid, angle.Key)
		if !CanSeeEXIF(c, svc.OrganizationID) {
			redactEXIF(&out.Photo)
		}
		return s.emitCompleted(ctx, q, tx, c, svc, missingBefore)
	})
	return out, err
}

func (s *Service) DeletePhoto(ctx context.Context, c Caller, serviceID uuid.UUID, angleKey string) (string, error) {
	var key string
	err := s.inTx(ctx, func(q *db.Queries, _ pgx.Tx) error {
		svc, err := s.lockWritableService(ctx, q, c, serviceID)
		if err != nil {
			return err
		}
		executed, err := q.HasExecutedServiceContract(ctx, svc.ID)
		if err != nil {
			return fmt.Errorf("photo_standard: contract: %w", err)
		}
		if executed {
			return ErrServiceLocked
		}
		angle, err := q.GetPhotoAngleByKey(ctx, db.GetPhotoAngleByKeyParams{Key: strings.TrimSpace(angleKey), BrandID: svc.BrandID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("photo_standard: angle: %w", err)
		}
		old, err := q.SoftDeleteActiveIntakePhoto(ctx, db.SoftDeleteActiveIntakePhotoParams{
			ServiceID: svc.ID, AngleID: angle.ID, DeletedBy: int8OrNull(c.Principal.UserInternal),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("photo_standard: delete photo: %w", err)
		}
		key = old.StorageKey
		return nil
	})
	return key, err
}

func (s *Service) inTx(ctx context.Context, fn func(*db.Queries, pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("photo_standard: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx), tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("photo_standard: commit: %w", err)
	}
	return nil
}

func (s *Service) visibleService(ctx context.Context, c Caller, id uuid.UUID) (db.Service, error) {
	svc, err := s.q.GetServiceByUUID(ctx, db.GetServiceByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Service{}, ErrNotFound
	}
	if err != nil {
		return db.Service{}, fmt.Errorf("photo_standard: service: %w", err)
	}
	if !c.Filter.AllowsOrg(svc.OrganizationID, svc.BrandID) {
		return db.Service{}, ErrNotFound
	}
	return svc, nil
}

func (s *Service) lockWritableService(ctx context.Context, q *db.Queries, c Caller, id uuid.UUID) (db.Service, error) {
	svc, err := q.LockServiceByUUID(ctx, db.LockServiceByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Service{}, ErrNotFound
	}
	if err != nil {
		return db.Service{}, fmt.Errorf("photo_standard: lock service: %w", err)
	}
	if !c.Filter.AllowsOrg(svc.OrganizationID, svc.BrandID) {
		return db.Service{}, ErrNotFound
	}
	if svc.Status == "completed" || svc.Status == "cancelled" {
		return db.Service{}, ErrServiceLocked
	}
	return svc, nil
}

func (s *Service) distributorID(ctx context.Context, orgID int64) (pgtype.Int8, error) {
	org, err := s.q.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return pgtype.Int8{}, fmt.Errorf("photo_standard: org: %w", err)
	}
	if org.Type == "distributor" {
		return pgtype.Int8{Int64: org.ID, Valid: true}, nil
	}
	if org.Type != "dealer" || !org.ParentID.Valid {
		return pgtype.Int8{}, nil
	}
	parent, err := s.q.GetOrganizationByID(ctx, org.ParentID.Int64)
	if err != nil {
		return pgtype.Int8{}, fmt.Errorf("photo_standard: parent org: %w", err)
	}
	if parent.Type == "distributor" {
		return pgtype.Int8{Int64: parent.ID, Valid: true}, nil
	}
	return pgtype.Int8{}, nil
}

func validateAngle(in AngleInput) error {
	if strings.TrimSpace(in.Key) == "" || len(strings.TrimSpace(in.Key)) > 64 {
		return ErrValidation
	}
	if !json.Valid(in.Name) || string(in.Name) == "{}" {
		return ErrValidation
	}
	if len(in.Hint) > 0 && !json.Valid(in.Hint) {
		return ErrValidation
	}
	return nil
}

func Digest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func angleView(row db.PhotoAngle, required, hidden bool) AngleView {
	return AngleView{
		UUID: row.Uuid, Key: row.Key, Name: copyJSON(row.Name), Hint: copyJSON(row.Hint),
		ExampleStorageKey: textPtr(row.ExampleStorageKey), ExampleURL: exampleURL(row.Uuid, row.ExampleStorageKey),
		Required: required, Hidden: hidden, SortOrder: row.SortOrder, Active: row.Active,
	}
}

// ExampleURL is the authenticated path of an angle's example image.
func ExampleURL(angleUUID uuid.UUID) string {
	return fmt.Sprintf("/v1/photo-standard/angles/%s/example", angleUUID.String())
}

func exampleURL(angleUUID uuid.UUID, key pgtype.Text) string {
	if !key.Valid {
		return ""
	}
	return ExampleURL(angleUUID)
}

func photoView(row db.IntakePhoto, serviceUUID uuid.UUID, angleKey string) IntakePhotoView {
	return IntakePhotoView{
		UUID: row.Uuid, AngleKey: angleKey, URL: IntakePhotoURL(serviceUUID, angleKey),
		Mime: row.Mime, Size: row.Size, SHA256: row.Sha256, Width: int4Ptr(row.Width),
		Height: int4Ptr(row.Height), ExifTakenAt: timePtr(row.ExifTakenAt),
		ExifLat: numericPtr(row.ExifLat), ExifLng: numericPtr(row.ExifLng),
		ExifDevice: textPtr(row.ExifDevice), CreatedAt: row.CreatedAt.Time,
	}
}

func IntakePhotoURL(serviceUUID uuid.UUID, angleKey string) string {
	return fmt.Sprintf("/v1/services/%s/intake-photos/%s/file", serviceUUID.String(), angleKey)
}

func copyJSON(b []byte) json.RawMessage {
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

func hintOrEmpty(b json.RawMessage) []byte {
	if len(b) == 0 {
		return []byte(`{}`)
	}
	return []byte(b)
}

func textOrNull(s *string) pgtype.Text {
	if s == nil || strings.TrimSpace(*s) == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: strings.TrimSpace(*s), Valid: true}
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func int8OrNull(v int64) pgtype.Int8 {
	if v == 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: v, Valid: true}
}

func int4OrNull(v *int32) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *v, Valid: true}
}

func int4Ptr(v pgtype.Int4) *int32 {
	if !v.Valid {
		return nil
	}
	return &v.Int32
}

func timeOrNull(v *time.Time) pgtype.Timestamptz {
	if v == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *v, Valid: true}
}

func timePtr(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	return &v.Time
}

func numericOrNull(v *string) pgtype.Numeric {
	if v == nil || strings.TrimSpace(*v) == "" {
		return pgtype.Numeric{}
	}
	var n pgtype.Numeric
	_ = n.Scan(strings.TrimSpace(*v))
	return n
}

func numericPtr(v pgtype.Numeric) *string {
	if !v.Valid {
		return nil
	}
	raw, err := v.MarshalJSON()
	if err != nil {
		return nil
	}
	s := string(raw)
	return &s
}

func textUUID(v uuid.UUID) uuid.UUID { return v }

var _ = rbac.ScopeManaged

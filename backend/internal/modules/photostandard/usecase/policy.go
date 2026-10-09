package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"image"
	"image/jpeg"
	_ "image/png" // decoder for intake photos
	"io"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/photostandard/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/jackc/pgx/v5"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

// TEC-499 (F5-07b): the photo standard rule, the intake photo grid of the
// contract PDF and the KVKK handling of EXIF data.

// PDFPhotoMaxPx bounds the longer side of an intake photo embedded in a PDF.
const PDFPhotoMaxPx = 640

// FeatureChecker reports whether a module is enabled for an organization.
type FeatureChecker interface {
	Enabled(ctx context.Context, organizationID int64, key string) (bool, error)
}

// WithFeatures wires the module resolver of the photo standard rule. Without
// it the rule never blocks.
func (s *Service) WithFeatures(f FeatureChecker) *Service {
	s.features = f
	return s
}

// WithOutbox wires the service.intake_photos_completed event.
func (s *Service) WithOutbox(out outbox.Enqueuer) *Service {
	s.out = out
	return s
}

// WithStorage wires the object store the PDF grid reads photos from.
func (s *Service) WithStorage(store storage.Driver) *Service {
	s.store = store
	return s
}

// RequireComplete is the intake rule: when photo_standard is on for the
// service organization, every required (resolved, not hidden) angle needs an
// active photo, otherwise *model.IncompleteError lists the missing keys. q
// may be a transaction; nil uses the pool.
func (s *Service) RequireComplete(ctx context.Context, q *db.Queries, ref model.ServiceRef) error {
	if s.features == nil {
		return nil
	}
	on, err := s.features.Enabled(ctx, ref.OrganizationID, features.ModulePhotoStandard)
	if err != nil {
		return fmt.Errorf("photo_standard: feature: %w", err)
	}
	if !on {
		return nil
	}
	if q == nil {
		q = s.q
	}
	missing, err := s.missingAngles(ctx, q, ref)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return &model.IncompleteError{Missing: missing}
	}
	return nil
}

func (s *Service) missingAngles(ctx context.Context, q *db.Queries, ref model.ServiceRef) ([]string, error) {
	angles, err := s.resolvedRows(ctx, q, ref.OrganizationID, ref.BrandID)
	if err != nil {
		return nil, err
	}
	photos, err := q.ListActiveIntakePhotosForService(ctx, ref.ID)
	if err != nil {
		return nil, fmt.Errorf("photo_standard: photos: %w", err)
	}
	have := make(map[int64]bool, len(photos))
	for _, p := range photos {
		have[p.AngleID] = true
	}
	missing := []string{}
	for _, a := range angles {
		if a.ResolvedRequired && !have[a.ID] {
			missing = append(missing, a.Key)
		}
	}
	return missing, nil
}

func (s *Service) resolvedRows(ctx context.Context, q *db.Queries, orgID, brandID int64) ([]db.ListResolvedPhotoAnglesRow, error) {
	distID, err := s.distributorID(ctx, orgID)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListResolvedPhotoAngles(ctx, db.ListResolvedPhotoAnglesParams{
		BrandID: brandID, ServiceOrgID: orgID, DistributorOrgID: distID,
	})
	if err != nil {
		return nil, fmt.Errorf("photo_standard: resolved angles: %w", err)
	}
	out := rows[:0]
	for _, r := range rows {
		if !r.ResolvedHidden {
			out = append(out, r)
		}
	}
	return out, nil
}

// emitCompleted writes service.intake_photos_completed when the upload in tx
// photographed the last missing required angle (missingBefore non-empty,
// nothing missing now). Uploads on a complete set (replacements) emit
// nothing, so the event fires once per completion.
func (s *Service) emitCompleted(ctx context.Context, q *db.Queries, tx pgx.Tx, c Caller, svc db.Service, missingBefore []string) error {
	if s.out == nil || len(missingBefore) == 0 {
		return nil
	}
	ref := model.ServiceRef{ID: svc.ID, OrganizationID: svc.OrganizationID, BrandID: svc.BrandID}
	missing, err := s.missingAngles(ctx, q, ref)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return nil
	}
	angles, err := s.resolvedRows(ctx, q, svc.OrganizationID, svc.BrandID)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(angles))
	for _, a := range angles {
		if a.ResolvedRequired {
			keys = append(keys, a.Key)
		}
	}
	id, uid := svc.ID, svc.Uuid
	ev := events.New(events.ServiceIntakePhotosCompleted).
		WithTenant(svc.OrganizationID).
		WithEntity("service", &id, &uid).
		WithPayload(map[string]any{
			"service_id": svc.ID, "service_uuid": svc.Uuid.String(),
			"organization_id": svc.OrganizationID, "brand_id": svc.BrandID, "angle_keys": keys,
		})
	if c.Principal.UserInternal != 0 {
		ev = ev.WithActor(c.Principal.UserInternal)
	}
	if err := s.out.Enqueue(ctx, tx, ev); err != nil {
		return fmt.Errorf("photo_standard: completed event: %w", err)
	}
	return nil
}

// CanSeeEXIF reports whether the caller may see EXIF location and device of
// the service's intake photos (KVKK): super admin, the brand center, or an
// owner of the service's own dealer organization.
func CanSeeEXIF(c Caller, serviceOrgID int64) bool {
	if c.Principal.IsSuperAdmin || c.Org.OrgType == rbac.OrgTypeCenter {
		return true
	}
	return c.Org.InternalID == serviceOrgID && c.Principal.HasRole(rbac.RoleDealerOwner)
}

func redactEXIF(v *IntakePhotoView) {
	v.ExifLat, v.ExifLng, v.ExifDevice = nil, nil, nil
}

// IntakePhotosHTML renders the active intake photos of a service as the
// {{intake_photos_html}} grid: one figure per photographed angle (angle
// order), captioned with the angle name in locale and the capture time
// (EXIF, else upload time). Images are decoded, downscaled to PDFPhotoMaxPx
// and re-encoded as JPEG data URLs, which also drops every EXIF block: no
// location or device reaches the PDF. Empty when there is no photo.
func (s *Service) IntakePhotosHTML(ctx context.Context, q *db.Queries, ref model.ServiceRef, locale string) (string, error) {
	if q == nil {
		q = s.q
	}
	photos, err := q.ListActiveIntakePhotosForService(ctx, ref.ID)
	if err != nil {
		return "", fmt.Errorf("photo_standard: photos: %w", err)
	}
	if len(photos) == 0 {
		return "", nil
	}
	byAngle := make(map[int64]db.IntakePhoto, len(photos))
	for _, p := range photos {
		byAngle[p.AngleID] = p
	}
	angles, err := q.ListPhotoAnglesByBrand(ctx, ref.BrandID)
	if err != nil {
		return "", fmt.Errorf("photo_standard: angles: %w", err)
	}
	var b strings.Builder
	b.WriteString(`<section class="doc-intake-photos"><h2>`)
	b.WriteString(html.EscapeString(intakeHeading(locale)))
	b.WriteString(`</h2><div class="doc-photos">`)
	for _, a := range angles {
		p, ok := byAngle[a.ID]
		if !ok {
			continue
		}
		name := AngleName(a.Name, locale, a.Key)
		taken := p.CreatedAt.Time
		if p.ExifTakenAt.Valid {
			taken = p.ExifTakenAt.Time
		}
		b.WriteString(`<figure><figcaption><strong>`)
		b.WriteString(html.EscapeString(name))
		b.WriteString(`</strong><br>`)
		b.WriteString(html.EscapeString(taken.UTC().Format("2006-01-02 15:04") + " UTC"))
		b.WriteString(`</figcaption>`)
		if data := s.pdfImage(ctx, p); data != nil {
			b.WriteString(pdfrender.ImageTag("image/jpeg", data, name))
		}
		b.WriteString(`</figure>`)
	}
	b.WriteString(`</div></section>`)
	return b.String(), nil
}

// pdfImage loads and downscales one photo; nil when storage is missing or
// the image cannot be decoded (HEIC has no Go decoder: caption only).
func (s *Service) pdfImage(ctx context.Context, p db.IntakePhoto) []byte {
	if s.store == nil || p.Mime == "image/heic" {
		return nil
	}
	rc, _, err := s.store.Download(ctx, p.StorageKey)
	if err != nil {
		return nil
	}
	defer func() { _ = rc.Close() }()
	raw, err := io.ReadAll(io.LimitReader(rc, MaxUploadBytes+1))
	if err != nil {
		return nil
	}
	out, err := DownscaleJPEG(raw, p.Mime, PDFPhotoMaxPx)
	if err != nil {
		return nil
	}
	return out
}

// DownscaleJPEG decodes a JPEG, PNG or WebP image, fits it into maxPx on the
// longer side and re-encodes it as a metadata-free JPEG.
func DownscaleJPEG(raw []byte, mime string, maxPx int) ([]byte, error) {
	var (
		src image.Image
		err error
	)
	if mime == "image/webp" {
		src, err = webp.Decode(bytes.NewReader(raw))
	} else {
		src, _, err = image.Decode(bytes.NewReader(raw))
	}
	if err != nil {
		return nil, err
	}
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("photo_standard: empty image")
	}
	if w > maxPx || h > maxPx {
		if w >= h {
			h = h * maxPx / w
			w = maxPx
		} else {
			w = w * maxPx / h
			h = maxPx
		}
		w, h = max(w, 1), max(h, 1)
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, bounds, xdraw.Src, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 80}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// AngleName picks the angle label in locale (then tr, en, any), falling back
// to the key.
func AngleName(raw []byte, locale, key string) string {
	names := map[string]string{}
	if err := json.Unmarshal(raw, &names); err != nil || len(names) == 0 {
		return key
	}
	loc := strings.ToLower(strings.TrimSpace(locale))
	candidates := []string{loc}
	if i := strings.IndexAny(loc, "-_"); i > 0 {
		candidates = append(candidates, loc[:i])
	}
	candidates = append(candidates, "tr", "en")
	for _, c := range candidates {
		if v := strings.TrimSpace(names[c]); v != "" {
			return v
		}
	}
	best := ""
	for k, v := range names {
		if strings.TrimSpace(v) != "" && (best == "" || k < best) {
			best = k
		}
	}
	if best != "" {
		return names[best]
	}
	return key
}

var intakeHeadings = map[string]string{
	"tr": "Araç kabul fotoğrafları", "en": "Vehicle intake photos", "bg": "Снимки при приемане на автомобила",
	"de": "Fahrzeugannahme-Fotos", "el": "Φωτογραφίες παραλαβής οχήματος", "uk": "Фото приймання автомобіля",
	"ru": "Фото приёмки автомобиля", "fr": "Photos de réception du véhicule", "es": "Fotos de recepción del vehículo",
	"it": "Foto di accettazione del veicolo", "zh": "车辆接收照片", "az": "Avtomobilin qəbul fotoşəkilləri",
	"ar": "صور استلام المركبة",
}

func intakeHeading(locale string) string {
	loc := strings.ToLower(strings.TrimSpace(locale))
	if i := strings.IndexAny(loc, "-_"); i > 0 {
		loc = loc[:i]
	}
	if v, ok := intakeHeadings[loc]; ok {
		return v
	}
	return intakeHeadings["en"]
}

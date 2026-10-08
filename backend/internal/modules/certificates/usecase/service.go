package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const MaxPDFBytes int64 = 10 << 20

var (
	ErrNotFound                    = errors.New("certificates: not found")
	ErrForbidden                   = errors.New("certificates: forbidden")
	ErrStorageRequired             = errors.New("certificates: storage required")
	ErrUnsupportedMediaType        = errors.New("certificates: unsupported media type")
	ErrFileTooLarge                = errors.New("certificates: file too large")
	ErrCertificateApprovalRequired = usecase.ErrCertificateApprovalRequired
)

type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type FeatureChecker interface {
	Enabled(ctx context.Context, organizationID int64, key string) (bool, error)
}

type Settings interface {
	CertificatesRequireAdminApproval(ctx context.Context) bool
	CertificatesExpiryNoticeDays(ctx context.Context) int
}

type Caller struct {
	Principal authctx.Principal
	Org       orgctx.Scope
	Filter    scopefilter.Filter
}

type Service struct {
	pool     TxBeginner
	q        *db.Queries
	store    storage.Driver
	out      outbox.Enqueuer
	features FeatureChecker
	settings Settings
	now      func() time.Time
}

func New(pool TxBeginner, q *db.Queries, store storage.Driver, out outbox.Enqueuer, features FeatureChecker, settings Settings) *Service {
	return &Service{pool: pool, q: q, store: store, out: out, features: features, settings: settings, now: time.Now}
}

func (s *Service) inTx(ctx context.Context, fn func(*db.Queries, pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	if err := fn(q, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) enabled(ctx context.Context, orgID int64) bool {
	if s.features == nil {
		return false
	}
	ok, err := s.features.Enabled(ctx, orgID, features.ModuleCertificates)
	return err == nil && ok
}

func actor(c Caller) pgtype.Int8 {
	if c.Principal.UserInternal == 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: c.Principal.UserInternal, Valid: true}
}

func text(v string) pgtype.Text {
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func (s *Service) emit(ctx context.Context, tx pgx.Tx, name string, orgID int64, entity string, id int64, uid uuid.UUID, actorID int64, payload map[string]any) error {
	if s.out == nil {
		return nil
	}
	ev := events.New(name).WithTenant(orgID).WithEntity(entity, &id, &uid).WithPayload(payload)
	if actorID != 0 {
		ev = ev.WithActor(actorID)
	}
	return s.out.Enqueue(ctx, tx, ev)
}

func (c Caller) can(slug string, orgID, brandID int64) bool {
	scope, ok := c.Principal.ScopeFor(slug)
	if !ok || brandID != c.Org.BrandID {
		return false
	}
	switch scope {
	case rbac.ScopeAll, rbac.ScopeBrand:
		return true
	case rbac.ScopeSubtree:
		return c.Filter.AllowsOrg(orgID, brandID)
	case rbac.ScopeManaged:
		return orgID == c.Org.InternalID
	}
	return false
}

type CertificateView struct {
	UUID             string     `json:"uuid"`
	UserUUID         string     `json:"user_uuid,omitempty"`
	TypeUUID         string     `json:"type_uuid,omitempty"`
	OrganizationUUID string     `json:"organization_uuid,omitempty"`
	Status           string     `json:"status"`
	SHA256           string     `json:"sha256,omitempty"`
	StorageKey       string     `json:"storage_key,omitempty"`
	IssuedAt         *time.Time `json:"issued_at,omitempty"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

func certView(c db.Certificate) CertificateView {
	return CertificateView{
		UUID: c.Uuid.String(), Status: c.Status, SHA256: c.Sha256, StorageKey: c.StorageKey,
		IssuedAt: timePtr(c.IssuedAt), ExpiresAt: timePtr(c.ExpiresAt), CreatedAt: c.CreatedAt.Time,
	}
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

type UploadInput struct {
	UserUUID  uuid.UUID
	TypeUUID  uuid.UUID
	IssuedAt  time.Time
	ExpiresAt *time.Time
	Filename  string
	Body      io.Reader
	Size      int64
}

func (s *Service) Upload(ctx context.Context, c Caller, in UploadInput) (CertificateView, error) {
	if s.store == nil {
		return CertificateView{}, ErrStorageRequired
	}
	if in.Size <= 0 || in.Size > MaxPDFBytes {
		return CertificateView{}, ErrFileTooLarge
	}
	limited := io.LimitReader(in.Body, MaxPDFBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return CertificateView{}, err
	}
	if int64(len(raw)) > MaxPDFBytes {
		return CertificateView{}, ErrFileTooLarge
	}
	if len(raw) < 5 || string(raw[:5]) != "%PDF-" {
		return CertificateView{}, ErrUnsupportedMediaType
	}
	sum := sha256.Sum256(raw)
	sha := hex.EncodeToString(sum[:])
	var out CertificateView
	err = s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		user, err := q.GetUserByUUID(ctx, in.UserUUID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		typ, err := q.GetCertificateTypeByUUID(ctx, db.GetCertificateTypeByUUIDParams{Uuid: in.TypeUUID, BrandID: c.Org.BrandID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		org, err := q.GetOrganizationByID(ctx, c.Org.InternalID)
		if err != nil {
			return err
		}
		if !c.can(rbac.PermCertificatesWrite, org.ID, org.BrandID) {
			return ErrForbidden
		}
		params := db.CreateCertificateParams{
			UserID: user.ID, OrganizationID: org.ID, BrandID: org.BrandID, TypeID: typ.ID,
			StorageKey: "certificates/" + org.Uuid.String() + "/pending-" + sha + ".pdf",
			Sha256:     sha, IssuedAt: ts(in.IssuedAt), Status: "pending",
		}
		if in.ExpiresAt != nil {
			params.ExpiresAt = ts(*in.ExpiresAt)
		}
		row, err := q.CreateCertificate(ctx, params)
		if err != nil {
			return err
		}
		key := "certificates/" + org.Uuid.String() + "/" + row.Uuid.String() + ".pdf"
		if err := s.store.Upload(ctx, storage.File{
			Body: bytesReader(raw), Size: int64(len(raw)), ContentType: "application/pdf", Filename: in.Filename,
		}, key); err != nil {
			return err
		}
		row, err = q.UpdateCertificateStorageKey(ctx, db.UpdateCertificateStorageKeyParams{ID: row.ID, BrandID: row.BrandID, StorageKey: key})
		if err != nil {
			return err
		}
		if err := s.emit(ctx, tx, events.CertificateUploaded, row.OrganizationID, "certificate", row.ID, row.Uuid, c.Principal.UserInternal, map[string]any{
			"certificate_uuid": row.Uuid.String(), "type_id": row.TypeID, "user_id": row.UserID,
		}); err != nil {
			return err
		}
		out = certView(row)
		return nil
	})
	return out, err
}

func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }

func (s *Service) Verify(ctx context.Context, c Caller, id uuid.UUID) (CertificateView, error) {
	return s.status(ctx, c, id, "valid", "")
}

func (s *Service) Reject(ctx context.Context, c Caller, id uuid.UUID, reason string) (CertificateView, error) {
	if reason == "" {
		reason = "rejected"
	}
	return s.status(ctx, c, id, "rejected", reason)
}

func (s *Service) Revoke(ctx context.Context, c Caller, id uuid.UUID) (CertificateView, error) {
	return s.status(ctx, c, id, "revoked", "")
}

func (s *Service) status(ctx context.Context, c Caller, id uuid.UUID, status, reason string) (CertificateView, error) {
	var out CertificateView
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		row, err := q.GetCertificateByUUID(ctx, db.GetCertificateByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !c.can(rbac.PermCertificatesVerify, row.OrganizationID, row.BrandID) {
			return ErrNotFound
		}
		now := ts(s.now().UTC())
		switch status {
		case "valid":
			row, err = q.VerifyCertificate(ctx, db.VerifyCertificateParams{
				ID: row.ID, BrandID: row.BrandID, VerifiedByUserID: actor(c),
				VerifiedByOrgID: pgtype.Int8{Int64: c.Org.InternalID, Valid: c.Org.InternalID != 0}, VerifiedAt: now,
			})
		case "rejected":
			row, err = q.RejectCertificate(ctx, db.RejectCertificateParams{
				ID: row.ID, BrandID: row.BrandID, VerifiedByUserID: actor(c),
				VerifiedByOrgID: pgtype.Int8{Int64: c.Org.InternalID, Valid: c.Org.InternalID != 0},
				VerifiedAt:      now, RejectReason: text(reason),
			})
		case "revoked":
			row, err = q.RevokeCertificate(ctx, db.RevokeCertificateParams{ID: row.ID, BrandID: row.BrandID})
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		name := map[string]string{"valid": events.CertificateVerified, "rejected": events.CertificateRejected, "revoked": events.CertificateRevoked}[status]
		if err := s.emit(ctx, tx, name, row.OrganizationID, "certificate", row.ID, row.Uuid, c.Principal.UserInternal, map[string]any{
			"certificate_uuid": row.Uuid.String(), "status": row.Status,
		}); err != nil {
			return err
		}
		out = certView(row)
		return nil
	})
	return out, err
}

func (s *Service) Download(ctx context.Context, c Caller, id uuid.UUID) (io.ReadCloser, int64, error) {
	if s.store == nil {
		return nil, 0, ErrStorageRequired
	}
	row, err := s.q.GetCertificateByUUID(ctx, db.GetCertificateByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, ErrNotFound
	}
	if err != nil {
		return nil, 0, err
	}
	if !c.can(rbac.PermCertificatesRead, row.OrganizationID, row.BrandID) {
		return nil, 0, ErrNotFound
	}
	return s.store.Download(ctx, row.StorageKey)
}

func (s *Service) EvaluateService(ctx context.Context, q *db.Queries, tx pgx.Tx, svc db.Service, actorUserID int64) error {
	if !s.enabled(ctx, svc.OrganizationID) {
		return nil
	}
	userID := actorUserID
	if svc.PerformedByUserID.Valid {
		userID = svc.PerformedByUserID.Int64
	} else if svc.CreatedByUserID.Valid {
		userID = svc.CreatedByUserID.Int64
	}
	if userID == 0 {
		return nil
	}
	required, err := q.ListRequiredCertificateTypesForService(ctx, db.ListRequiredCertificateTypesForServiceParams{
		BrandID: svc.BrandID, ServiceID: svc.ID,
	})
	if err != nil {
		return err
	}
	if len(required) == 0 {
		return nil
	}
	validRows, err := q.ListValidCertificatesForServiceUser(ctx, db.ListValidCertificatesForServiceUserParams{
		BrandID: svc.BrandID, OrganizationID: svc.OrganizationID, UserID: userID, ServiceID: svc.ID, Now: ts(s.now().UTC()),
	})
	if err != nil {
		return err
	}
	valid := map[int64]bool{}
	for _, row := range validRows {
		valid[row.TypeID] = true
	}
	for _, typ := range required {
		if valid[typ.ID] {
			continue
		}
		reason := "missing"
		latest, err := q.LatestCertificateForUserType(ctx, db.LatestCertificateForUserTypeParams{
			UserID: userID, OrganizationID: svc.OrganizationID, BrandID: svc.BrandID, TypeID: typ.ID,
		})
		if err == nil {
			switch latest.Status {
			case "expired":
				reason = "expired"
			case "pending":
				reason = "pending"
			case "valid":
				if latest.ExpiresAt.Valid && !latest.ExpiresAt.Time.After(s.now()) {
					reason = "expired"
				}
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		w, err := q.UpsertServiceCertificateWarning(ctx, db.UpsertServiceCertificateWarningParams{
			ServiceID: svc.ID, OrganizationID: svc.OrganizationID, BrandID: svc.BrandID,
			UserID: userID, TypeID: typ.ID, Reason: reason, Decision: "none", Note: "",
		})
		if err != nil {
			return err
		}
		if err := s.emit(ctx, tx, events.CertificateServiceWarning, svc.OrganizationID, "service_certificate_warning", w.ID, w.Uuid, actorUserID, map[string]any{
			"service_uuid": svc.Uuid.String(), "warning_uuid": w.Uuid.String(), "reason": reason,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) BeforeStatusChange(ctx context.Context, q *db.Queries, tx pgx.Tx, svc db.Service, actorUserID int64) error {
	if !s.enabled(ctx, svc.OrganizationID) {
		return nil
	}
	if err := s.EvaluateService(ctx, q, tx, svc, actorUserID); err != nil {
		return err
	}
	if s.settings == nil || !s.settings.CertificatesRequireAdminApproval(ctx) {
		return nil
	}
	if _, err := q.MarkServiceCertificateWarningsPending(ctx, db.MarkServiceCertificateWarningsPendingParams{ServiceID: svc.ID, BrandID: svc.BrandID}); err != nil {
		return err
	}
	n, err := q.CountBlockingServiceCertificateWarnings(ctx, db.CountBlockingServiceCertificateWarningsParams{ServiceID: svc.ID, BrandID: svc.BrandID})
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrCertificateApprovalRequired
	}
	return nil
}

type WarningDecisionInput struct {
	Note string
}

func (s *Service) DecideWarning(ctx context.Context, c Caller, id uuid.UUID, approve bool, note string) (db.ServiceCertificateWarning, error) {
	var out db.ServiceCertificateWarning
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		found, err := q.GetServiceCertificateWarningByUUID(ctx, db.GetServiceCertificateWarningByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !c.can(rbac.PermCertificatesApproveService, found.OrganizationID, found.BrandID) {
			return ErrNotFound
		}
		decision := "rejected"
		if approve {
			decision = "approved"
		}
		row, err := q.DecideServiceCertificateWarning(ctx, db.DecideServiceCertificateWarningParams{
			ID: found.ID, BrandID: found.BrandID, Decision: decision, DecidedBy: actor(c), DecidedAt: ts(s.now().UTC()), Note: note,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		out = row
		return nil
	})
	return out, err
}

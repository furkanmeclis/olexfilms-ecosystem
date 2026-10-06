// Package usecase implements warranty claim workflows.
package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty_claims/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	platstorage "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	StatusOpen         = "open"
	StatusDealerReview = "dealer_review"
	StatusCenterReview = "center_review"
	StatusApproved     = "approved"
	StatusRejected     = "rejected"

	CodePhotoRequired = "CLAIM_PHOTO_REQUIRED"

	MaxPhotoBytes = 12 << 20

	EventStatusChanged = "warranty_claim.status_changed"
)

var (
	ErrForbidden       = errors.New("warranty claims: forbidden")
	ErrNotFound        = errors.New("warranty claims: not found")
	ErrInvalidRequest  = errors.New("warranty claims: invalid request")
	ErrPhotoRequired   = errors.New("warranty claims: photo required")
	ErrConflict        = errors.New("warranty claims: conflict")
	ErrUnsupportedFlow = errors.New("warranty claims: unsupported transition")
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

func invalid(field, message string) error { return &ValidationError{Field: field, Message: message} }

type Store interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	CreateWarrantyClaim(ctx context.Context, arg db.CreateWarrantyClaimParams) (db.WarrantyClaim, error)
	GetWarrantyByUUID(ctx context.Context, arg db.GetWarrantyByUUIDParams) (db.Warranty, error)
	GetWarrantyClaimByUUID(ctx context.Context, arg db.GetWarrantyClaimByUUIDParams) (db.WarrantyClaim, error)
	GetWarrantyClaimByUUIDForUpdate(ctx context.Context, arg db.GetWarrantyClaimByUUIDForUpdateParams) (db.WarrantyClaim, error)
	GetWarrantyClaimCoverageContext(ctx context.Context, arg db.GetWarrantyClaimCoverageContextParams) (db.GetWarrantyClaimCoverageContextRow, error)
	GetWarrantyClaimOpenContext(ctx context.Context, arg db.GetWarrantyClaimOpenContextParams) (db.GetWarrantyClaimOpenContextRow, error)
	GetServiceItemByUUID(ctx context.Context, arg db.GetServiceItemByUUIDParams) (db.ServiceItem, error)
	GetServiceByUUID(ctx context.Context, arg db.GetServiceByUUIDParams) (db.Service, error)
	GetVehicleByUUID(ctx context.Context, argUuid uuid.UUID) (db.Vehicle, error)
	AddWarrantyClaimPart(ctx context.Context, arg db.AddWarrantyClaimPartParams) (db.WarrantyClaimPart, error)
	AddWarrantyClaimPhoto(ctx context.Context, arg db.AddWarrantyClaimPhotoParams) (db.WarrantyClaimPhoto, error)
	CountWarrantyClaimPhotos(ctx context.Context, claimID int64) (int64, error)
	ListWarrantyClaimParts(ctx context.Context, claimID int64) ([]db.WarrantyClaimPart, error)
	ListWarrantyClaimPhotos(ctx context.Context, claimID int64) ([]db.WarrantyClaimPhoto, error)
	ListWarrantyClaimEvents(ctx context.Context, claimID int64) ([]db.WarrantyClaimEvent, error)
	ListWarrantyClaimsInScope(ctx context.Context, arg db.ListWarrantyClaimsInScopeParams) ([]db.WarrantyClaim, error)
	CountWarrantyClaimsInScope(ctx context.Context, arg db.CountWarrantyClaimsInScopeParams) (int64, error)
	ListWarrantyClaimsByWarranty(ctx context.Context, arg db.ListWarrantyClaimsByWarrantyParams) ([]db.WarrantyClaim, error)
	SetWarrantyClaimStatus(ctx context.Context, arg db.SetWarrantyClaimStatusParams) (db.WarrantyClaim, error)
	ListWarrantyClaimNotifyUsersByOrg(ctx context.Context, arg db.ListWarrantyClaimNotifyUsersByOrgParams) ([]int64, error)
	ListWarrantyClaimCenterNotifyUsers(ctx context.Context, arg db.ListWarrantyClaimCenterNotifyUsersParams) ([]int64, error)
	Descendants(ctx context.Context, id int64) ([]db.Organization, error)
	GetOrganizationByID(ctx context.Context, id int64) (db.Organization, error)
	WarrantyClaimFailureRateByProduct(ctx context.Context, arg db.WarrantyClaimFailureRateByProductParams) ([]db.WarrantyClaimFailureRateByProductRow, error)
	WarrantyClaimFailureRateByLot(ctx context.Context, arg db.WarrantyClaimFailureRateByLotParams) ([]db.WarrantyClaimFailureRateByLotRow, error)
	WarrantyClaimsByDealerReport(ctx context.Context, arg db.WarrantyClaimsByDealerReportParams) ([]db.WarrantyClaimsByDealerReportRow, error)
	WarrantyClaimPartsReport(ctx context.Context, arg db.WarrantyClaimPartsReportParams) ([]db.WarrantyClaimPartsReportRow, error)
	GetBrandCenter(ctx context.Context, brandID int64) (db.Organization, error)
	ListWarrantyClaimCostEntries(ctx context.Context, arg db.ListWarrantyClaimCostEntriesParams) ([]db.ListWarrantyClaimCostEntriesRow, error)
}

type txBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type Storage interface {
	Upload(ctx context.Context, file platstorage.File, path string) error
}

type txStore struct {
	*db.Queries
	pool txBeginner
}

func (s txStore) Begin(ctx context.Context) (pgx.Tx, error) {
	if s.pool == nil {
		return nil, errors.New("warranty claims: transaction pool is nil")
	}
	return s.pool.Begin(ctx)
}

type Caller struct {
	UserID         int64
	OrganizationID int64
	BrandID        int64
	OrgType        string
	Filter         scopefilter.Filter
	Permissions    map[string]rbac.Scope
}

type Service struct {
	q       Store
	storage Storage
	out     outbox.Enqueuer
	now     func() time.Time
}

func New(pool *pgxpool.Pool, q *db.Queries, storage Storage, out outbox.Enqueuer) *Service {
	return &Service{q: txStore{Queries: q, pool: pool}, storage: storage, out: out, now: func() time.Time { return time.Now().UTC() }}
}

func NewWithStore(q Store, storage Storage, out outbox.Enqueuer) *Service {
	return &Service{q: q, storage: storage, out: out, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) WithClock(now func() time.Time) *Service {
	if now != nil {
		s.now = now
	}
	return s
}

func (s *Service) Create(ctx context.Context, c Caller, in model.CreateInput) (model.ClaimView, error) {
	if c.OrgType == "customer" || c.OrganizationID <= 0 {
		return model.ClaimView{}, ErrForbidden
	}
	desc := strings.TrimSpace(in.Description)
	if desc == "" {
		return model.ClaimView{}, invalid("description", "description is required")
	}
	if len(desc) > 20000 {
		return model.ClaimView{}, invalid("description", "description is too long")
	}
	if len(in.Parts) == 0 {
		return model.ClaimView{}, invalid("parts", "at least one part is required")
	}
	w, err := s.q.GetWarrantyByUUID(ctx, db.GetWarrantyByUUIDParams{Uuid: in.WarrantyUUID, BrandID: c.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.ClaimView{}, ErrNotFound
	}
	if err != nil {
		return model.ClaimView{}, fmt.Errorf("warranty claims: warranty: %w", err)
	}
	coverage, err := s.coverage(ctx, w, in.Parts)
	if err != nil {
		return model.ClaimView{}, err
	}
	body, err := json.Marshal(coverage)
	if err != nil {
		return model.ClaimView{}, err
	}
	tx, err := s.q.Begin(ctx)
	if err != nil {
		return model.ClaimView{}, fmt.Errorf("warranty claims: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := db.New(tx)
	var row db.WarrantyClaim
	for attempt := 0; attempt < 3; attempt++ {
		row, err = qtx.CreateWarrantyClaim(ctx, db.CreateWarrantyClaimParams{
			OrganizationID:  c.OrganizationID,
			BrandID:         c.BrandID,
			WarrantyID:      w.ID,
			ServiceID:       w.ServiceID,
			VehicleID:       w.VehicleID,
			CustomerUserID:  w.HolderUserID,
			Description:     desc,
			Status:          StatusOpen,
			CoverageCheck:   body,
			CreatedByUserID: int8(c.UserID),
		})
		if err == nil {
			break
		}
		if !isUnique(err, "uq_warranty_claims_org_no") {
			return model.ClaimView{}, mapCreateErr(err)
		}
	}
	if err != nil {
		return model.ClaimView{}, mapCreateErr(err)
	}
	for _, p := range in.Parts {
		if _, err := s.addPartWith(ctx, qtx, row, p); err != nil {
			return model.ClaimView{}, err
		}
	}
	if err := s.emitOpen(ctx, tx, qtx, row); err != nil {
		return model.ClaimView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.ClaimView{}, err
	}
	return s.view(ctx, row, true)
}

func (s *Service) AddPhoto(ctx context.Context, c Caller, id uuid.UUID, in PhotoInput) (model.PhotoView, error) {
	claim, err := s.claim(ctx, c, id, true)
	if err != nil {
		return model.PhotoView{}, err
	}
	data, mimeType, ext, sha, err := readPhoto(in)
	if err != nil {
		return model.PhotoView{}, err
	}
	photoID := uuid.New()
	key := fmt.Sprintf("warranty-claims/%s/photos/%s.%s", claim.Uuid.String(), photoID.String(), ext)
	if s.storage == nil {
		return model.PhotoView{}, errors.New("warranty claims: storage is nil")
	}
	if err := s.storage.Upload(ctx, platstorage.File{
		Body: bytes.NewReader(data), Size: int64(len(data)), ContentType: mimeType, Filename: in.Filename,
	}, key); err != nil {
		return model.PhotoView{}, fmt.Errorf("warranty claims: upload photo: %w", err)
	}
	row, err := s.q.AddWarrantyClaimPhoto(ctx, db.AddWarrantyClaimPhotoParams{
		ClaimID: claim.ID, OrganizationID: claim.OrganizationID, BrandID: claim.BrandID,
		StorageKey: key, MimeType: mimeType, SizeBytes: int64(len(data)), Sha256: sha,
		UploadedByUserID: int8(c.UserID),
	})
	if err != nil {
		return model.PhotoView{}, mapDB(err)
	}
	return photoView(row), nil
}

type PhotoInput struct {
	Body     io.Reader
	Size     int64
	Filename string
}

func (s *Service) Transition(ctx context.Context, c Caller, id uuid.UUID, in model.TransitionInput) (model.ClaimView, error) {
	tx, err := s.q.Begin(ctx)
	if err != nil {
		return model.ClaimView{}, fmt.Errorf("warranty claims: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := db.New(tx)
	cur, err := qtx.GetWarrantyClaimByUUIDForUpdate(ctx, db.GetWarrantyClaimByUUIDForUpdateParams{
		Uuid: id, BrandID: c.BrandID, OrganizationIds: c.Filter.OrgIDsArg(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.ClaimView{}, ErrNotFound
	}
	if err != nil {
		return model.ClaimView{}, err
	}
	to, err := s.nextStatus(ctx, qtx, c, cur, strings.TrimSpace(in.Status))
	if err != nil {
		return model.ClaimView{}, err
	}
	reason := strings.TrimSpace(in.RejectionReason)
	if to == StatusRejected && reason == "" {
		return model.ClaimView{}, invalid("rejection_reason", "rejection reason is required")
	}
	if to != StatusRejected && reason != "" {
		return model.ClaimView{}, invalid("rejection_reason", "rejection reason is only accepted for rejection")
	}
	updated, err := qtx.SetWarrantyClaimStatus(ctx, db.SetWarrantyClaimStatusParams{
		ID: cur.ID, BrandID: cur.BrandID, FromStatus: cur.Status, Status: to,
		RejectionReason: text(reason), ActorUserID: int8(c.UserID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.ClaimView{}, ErrUnsupportedFlow
	}
	if err != nil {
		return model.ClaimView{}, mapDB(err)
	}
	if err := s.emitStatus(ctx, tx, updated, cur.Status, to); err != nil {
		return model.ClaimView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.ClaimView{}, fmt.Errorf("warranty claims: commit: %w", err)
	}
	return s.view(ctx, updated, true)
}

func (s *Service) List(ctx context.Context, c Caller, f model.ListFilter) (model.ListView, error) {
	args, err := s.listArgs(ctx, c, f)
	if err != nil {
		return model.ListView{}, err
	}
	rows, err := s.q.ListWarrantyClaimsInScope(ctx, args)
	if err != nil {
		return model.ListView{}, err
	}
	total, err := s.q.CountWarrantyClaimsInScope(ctx, countArgs(args))
	if err != nil {
		return model.ListView{}, err
	}
	out := make([]model.ClaimView, 0, len(rows))
	for _, r := range rows {
		v, err := s.view(ctx, r, false)
		if err != nil {
			return model.ListView{}, err
		}
		out = append(out, v)
	}
	return model.ListView{Items: out, Total: total, Limit: args.PageLimit, Offset: args.PageOffset}, nil
}

func (s *Service) PortalList(ctx context.Context, brandID, userID int64) ([]model.PortalClaimView, error) {
	if brandID <= 0 || userID <= 0 {
		return []model.PortalClaimView{}, nil
	}
	rows, err := s.q.ListWarrantyClaimsInScope(ctx, db.ListWarrantyClaimsInScopeParams{
		BrandID: brandID, OrganizationIds: nil, CustomerUserID: int8(userID), PageLimit: 100, PageOffset: 0,
		SortKey: "created_at", SortDesc: true,
	})
	if err != nil {
		return nil, err
	}
	out := make([]model.PortalClaimView, 0, len(rows))
	for _, r := range rows {
		out = append(out, portalView(r))
	}
	return out, nil
}

func (s *Service) PortalByWarranty(ctx context.Context, brandID, userID int64, warrantyID int64) ([]model.PortalClaimView, error) {
	rows, err := s.q.ListWarrantyClaimsByWarranty(ctx, db.ListWarrantyClaimsByWarrantyParams{WarrantyID: warrantyID, BrandID: brandID})
	if err != nil {
		return nil, err
	}
	out := []model.PortalClaimView{}
	for _, r := range rows {
		if r.CustomerUserID == userID {
			out = append(out, portalView(r))
		}
	}
	return out, nil
}

func (s *Service) FailureRateReport(ctx context.Context, c Caller, f model.ReportFilter) (model.FailureRateReport, error) {
	group, err := reportGroup(f.Group)
	if err != nil {
		return model.FailureRateReport{}, err
	}
	from, to := reportPeriod(f)
	if group == "lot" {
		rows, err := s.q.WarrantyClaimFailureRateByLot(ctx, db.WarrantyClaimFailureRateByLotParams{
			BrandID: c.BrandID, OrganizationIds: c.Filter.OrgIDsArg(), FromAt: from, ToAt: to,
		})
		if err != nil {
			return model.FailureRateReport{}, err
		}
		out := make([]model.FailureRateRow, 0, len(rows))
		for _, r := range rows {
			lotUUID := r.UnitUuid
			lotCode := r.LotCode
			out = append(out, model.FailureRateRow{
				Group: group, ProductUUID: r.ProductUuid, ProductSKU: r.ProductSku, ProductName: r.ProductName,
				LotUUID: &lotUUID, LotCode: &lotCode,
				WarrantyCount: r.WarrantyCount, ClaimCount: r.ClaimCount, ApprovedClaimCount: r.ApprovedClaimCount,
				ClaimRate: rate(r.ClaimCount, r.WarrantyCount), ApprovedRate: rate(r.ApprovedClaimCount, r.WarrantyCount),
			})
		}
		return model.FailureRateReport{Group: group, Items: out}, nil
	}
	rows, err := s.q.WarrantyClaimFailureRateByProduct(ctx, db.WarrantyClaimFailureRateByProductParams{
		BrandID: c.BrandID, OrganizationIds: c.Filter.OrgIDsArg(), FromAt: from, ToAt: to,
	})
	if err != nil {
		return model.FailureRateReport{}, err
	}
	out := make([]model.FailureRateRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, model.FailureRateRow{
			Group: group, ProductUUID: r.ProductUuid, ProductSKU: r.ProductSku, ProductName: r.ProductName,
			WarrantyCount: r.WarrantyCount, ClaimCount: r.ClaimCount, ApprovedClaimCount: r.ApprovedClaimCount,
			ClaimRate: rate(r.ClaimCount, r.WarrantyCount), ApprovedRate: rate(r.ApprovedClaimCount, r.WarrantyCount),
		})
	}
	return model.FailureRateReport{Group: group, Items: out}, nil
}

func (s *Service) ByDealerReport(ctx context.Context, c Caller, f model.ReportFilter) (model.DealerReport, error) {
	from, to := reportPeriod(f)
	rows, err := s.q.WarrantyClaimsByDealerReport(ctx, db.WarrantyClaimsByDealerReportParams{
		BrandID: c.BrandID, OrganizationIds: c.Filter.OrgIDsArg(), FromAt: from, ToAt: to,
	})
	if err != nil {
		return model.DealerReport{}, err
	}
	out := make([]model.DealerReportRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, model.DealerReportRow{
			OrganizationUUID: r.OrganizationUuid, OrganizationName: r.OrganizationName, OrganizationType: r.OrganizationType,
			ClaimCount: r.ClaimCount, ApprovedClaimCount: r.ApprovedClaimCount, RejectedClaimCount: r.RejectedClaimCount,
			ApprovalRate: rate(r.ApprovedClaimCount, r.ClaimCount),
		})
	}
	return model.DealerReport{Items: out}, nil
}

func (s *Service) PartsReport(ctx context.Context, c Caller, f model.ReportFilter) (model.PartsReport, error) {
	from, to := reportPeriod(f)
	rows, err := s.q.WarrantyClaimPartsReport(ctx, db.WarrantyClaimPartsReportParams{
		BrandID: c.BrandID, OrganizationIds: c.Filter.OrgIDsArg(), FromAt: from, ToAt: to,
	})
	if err != nil {
		return model.PartsReport{}, err
	}
	out := make([]model.PartsReportRow, 0, len(rows))
	for _, r := range rows {
		var productUUID *uuid.UUID
		if r.ProductUuid.Valid {
			v := uuid.UUID(r.ProductUuid.Bytes)
			productUUID = &v
		}
		out = append(out, model.PartsReportRow{
			PartKey: r.PartKey, ProductUUID: productUUID, ProductSKU: r.ProductSku, ProductName: r.ProductName,
			PartCount: r.PartCount, ClaimCount: r.ClaimCount, ApprovedClaimCount: r.ApprovedClaimCount,
		})
	}
	return model.PartsReport{Items: out}, nil
}

func (s *Service) claim(ctx context.Context, c Caller, id uuid.UUID, write bool) (db.WarrantyClaim, error) {
	row, err := s.q.GetWarrantyClaimByUUID(ctx, db.GetWarrantyClaimByUUIDParams{
		Uuid: id, BrandID: c.BrandID, OrganizationIds: c.Filter.OrgIDsArg(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.WarrantyClaim{}, ErrNotFound
	}
	if err != nil {
		return db.WarrantyClaim{}, err
	}
	if write && !c.Filter.AllowsOrg(row.OrganizationID, row.BrandID) {
		return db.WarrantyClaim{}, ErrForbidden
	}
	return row, nil
}

func (s *Service) nextStatus(ctx context.Context, q *db.Queries, c Caller, claim db.WarrantyClaim, requested string) (string, error) {
	switch requested {
	case StatusDealerReview:
		if !has(c, rbac.PermWarrantyClaimsWrite) {
			return "", ErrForbidden
		}
		if claim.Status != StatusOpen {
			return "", ErrUnsupportedFlow
		}
		n, err := q.CountWarrantyClaimPhotos(ctx, claim.ID)
		if err != nil {
			return "", err
		}
		if n == 0 {
			return "", ErrPhotoRequired
		}
		return StatusDealerReview, nil
	case StatusCenterReview:
		if claim.Status == StatusOpen && c.OrgType == "distributor" {
			if !has(c, rbac.PermWarrantyClaimsReview) {
				return "", ErrForbidden
			}
			return StatusCenterReview, nil
		}
		if claim.Status == StatusDealerReview {
			nc, err := q.GetWarrantyClaimOpenContext(ctx, db.GetWarrantyClaimOpenContextParams{ID: claim.ID, BrandID: claim.BrandID})
			if err != nil {
				return "", err
			}
			ok, err := canForwardDealerReviewToCenter(
				has(c, rbac.PermWarrantyClaimsReview),
				has(c, rbac.PermWarrantyClaimsWrite),
				nc.OrganizationParentType,
			)
			if err != nil {
				return "", err
			}
			if ok {
				return StatusCenterReview, nil
			}
		}
		return "", ErrUnsupportedFlow
	case StatusApproved, StatusRejected:
		if !has(c, rbac.PermWarrantyClaimsDecide) {
			return "", ErrForbidden
		}
		if claim.Status != StatusCenterReview {
			return "", ErrUnsupportedFlow
		}
		return requested, nil
	default:
		return "", invalid("status", "unsupported status")
	}
}

func canForwardDealerReviewToCenter(hasReview, hasWrite bool, parentType pgtype.Text) (bool, error) {
	if hasReview {
		return true, nil
	}
	if !hasWrite {
		return false, ErrForbidden
	}
	if !parentType.Valid || parentType.String == "center" {
		return true, nil
	}
	return false, ErrUnsupportedFlow
}

func has(c Caller, perm string) bool {
	_, ok := c.Permissions[perm]
	return ok
}

func (s *Service) addPartWith(ctx context.Context, q interface {
	GetServiceItemByUUID(context.Context, db.GetServiceItemByUUIDParams) (db.ServiceItem, error)
	AddWarrantyClaimPart(context.Context, db.AddWarrantyClaimPartParams) (db.WarrantyClaimPart, error)
}, claim db.WarrantyClaim, in model.PartInput) (db.WarrantyClaimPart, error) {
	part := strings.TrimSpace(in.PartKey)
	if !validPartKey(part) {
		return db.WarrantyClaimPart{}, invalid("parts.part_key", "invalid part key")
	}
	var serviceItemID, productID, unitID pgtype.Int8
	if in.ServiceItemUUID != nil {
		it, err := q.GetServiceItemByUUID(ctx, db.GetServiceItemByUUIDParams{Uuid: *in.ServiceItemUUID, ServiceID: claim.ServiceID})
		if errors.Is(err, pgx.ErrNoRows) {
			return db.WarrantyClaimPart{}, invalid("parts.service_item_uuid", "service item not found")
		}
		if err != nil {
			return db.WarrantyClaimPart{}, err
		}
		serviceItemID = int8(it.ID)
		productID = int8(it.ProductID)
		unitID = int8(it.UnitID)
	}
	return q.AddWarrantyClaimPart(ctx, db.AddWarrantyClaimPartParams{
		ClaimID: claim.ID, OrganizationID: claim.OrganizationID, BrandID: claim.BrandID,
		PartKey: part, ServiceItemID: serviceItemID, ProductID: productID, UnitID: unitID,
		Note: strings.TrimSpace(in.Note),
	})
}

func (s *Service) coverage(ctx context.Context, w db.Warranty, parts []model.PartInput) (model.CoverageCheck, error) {
	cc, err := s.q.GetWarrantyClaimCoverageContext(ctx, db.GetWarrantyClaimCoverageContextParams{WarrantyID: w.ID, BrandID: w.BrandID})
	if err != nil {
		return model.CoverageCheck{}, err
	}
	res := CheckCoverage(CoverageInput{
		Now: s.now(), WarrantyStatus: cc.WarrantyStatus, StartAt: cc.StartAt.Time, EndAt: cc.EndAt.Time,
		WarrantyServiceItemID: cc.WarrantyServiceItemID, ServiceItemProductID: cc.ServiceItemProductID,
		WarrantyDurationMonths: cc.WarrantyDurationMonths, AppliedPartsJSON: cc.AppliedParts,
		Parts: parts,
	})
	return res, nil
}

type CoverageInput struct {
	Now                    time.Time
	WarrantyStatus         string
	StartAt                time.Time
	EndAt                  time.Time
	WarrantyServiceItemID  int64
	ServiceItemProductID   int64
	WarrantyDurationMonths pgtype.Int4
	AppliedPartsJSON       []byte
	Parts                  []model.PartInput
}

func CheckCoverage(in CoverageInput) model.CoverageCheck {
	reasons := []string{}
	if in.WarrantyStatus == "void" || in.WarrantyStatus == "expired" {
		reasons = append(reasons, "warranty_not_active")
	}
	if !in.EndAt.IsZero() && !in.Now.Before(in.EndAt) {
		reasons = append(reasons, "warranty_period_expired")
	}
	if !in.WarrantyDurationMonths.Valid || in.WarrantyDurationMonths.Int32 <= 0 {
		reasons = append(reasons, "product_has_no_warranty")
	}
	applied := []string{}
	_ = json.Unmarshal(in.AppliedPartsJSON, &applied)
	appliedSet := map[string]bool{}
	for _, p := range applied {
		appliedSet[p] = true
	}
	for _, p := range in.Parts {
		part := strings.TrimSpace(p.PartKey)
		if part == "" || !appliedSet[part] {
			reasons = append(reasons, "part_not_covered")
			break
		}
	}
	return model.CoverageCheck{
		OK: len(reasons) == 0, Reasons: reasons,
		CheckedAt: in.Now.UTC().Format(time.RFC3339),
	}
}

func (s *Service) emitOpen(ctx context.Context, tx pgx.Tx, q *db.Queries, claim db.WarrantyClaim) error {
	if s.out == nil {
		return nil
	}
	nc, err := q.GetWarrantyClaimOpenContext(ctx, db.GetWarrantyClaimOpenContextParams{ID: claim.ID, BrandID: claim.BrandID})
	if err != nil {
		return err
	}
	recipients, err := s.openRecipients(ctx, q, nc)
	if err != nil {
		return err
	}
	ev := s.event(nc, StatusOpen, StatusOpen, recipients, "opened")
	return s.out.Enqueue(ctx, tx, ev)
}

func (s *Service) emitStatus(ctx context.Context, tx pgx.Tx, claim db.WarrantyClaim, from, to string) error {
	if s.out == nil {
		return nil
	}
	nc, err := db.New(tx).GetWarrantyClaimOpenContext(ctx, db.GetWarrantyClaimOpenContextParams{ID: claim.ID, BrandID: claim.BrandID})
	if err != nil {
		return err
	}
	recipients := []int64{}
	if to == StatusApproved || to == StatusRejected {
		recipients = append(recipients, nc.CustomerUserID)
	}
	orgUsers, err := db.New(tx).ListWarrantyClaimNotifyUsersByOrg(ctx, db.ListWarrantyClaimNotifyUsersByOrgParams{
		OrganizationIds: []int64{nc.OrganizationID}, PermissionSlug: rbac.PermWarrantyClaimsWrite,
	})
	if err != nil {
		return err
	}
	recipients = append(recipients, orgUsers...)
	ev := s.event(nc, from, to, uniquePositive(recipients), "status_changed")
	return s.out.Enqueue(ctx, tx, ev)
}

func (s *Service) openRecipients(ctx context.Context, q *db.Queries, nc db.GetWarrantyClaimOpenContextRow) ([]int64, error) {
	ids, err := q.ListWarrantyClaimCenterNotifyUsers(ctx, db.ListWarrantyClaimCenterNotifyUsersParams{
		BrandID: nc.BrandID, PermissionSlug: rbac.PermWarrantyClaimsDecide,
	})
	if err != nil {
		return nil, err
	}
	if nc.OrganizationParentID.Valid {
		dist, err := q.ListWarrantyClaimNotifyUsersByOrg(ctx, db.ListWarrantyClaimNotifyUsersByOrgParams{
			OrganizationIds: []int64{nc.OrganizationParentID.Int64}, PermissionSlug: rbac.PermWarrantyClaimsReview,
		})
		if err != nil {
			return nil, err
		}
		ids = append(ids, dist...)
	}
	return uniquePositive(ids), nil
}

func (s *Service) event(nc db.GetWarrantyClaimOpenContextRow, from, to string, recipients []int64, action string) events.Event {
	id := nc.ClaimID
	uid := nc.ClaimUuid
	payload := map[string]any{
		"brand_id":          nc.BrandID,
		"claim_uuid":        nc.ClaimUuid.String(),
		"claim_no":          nc.ClaimNo,
		"warranty_uuid":     nc.WarrantyUuid.String(),
		"public_code":       nc.PublicCode,
		"service_no":        nc.ServiceNo,
		"plate":             nc.Plate,
		"product_name":      nc.ProductName,
		"organization_name": nc.OrganizationName,
		"customer_user_id":  nc.CustomerUserID,
		"notify_user_ids":   recipients,
		"from":              from,
		"to":                to,
		"action":            action,
	}
	return events.New(EventStatusChanged).WithTenant(nc.OrganizationID).WithEntity("warranty_claim", &id, &uid).WithPayload(payload)
}

func (s *Service) listArgs(ctx context.Context, c Caller, f model.ListFilter) (db.ListWarrantyClaimsInScopeParams, error) {
	limit, offset := apiquery.Clamp(f.Limit, f.Offset, apiquery.DefaultLimit, apiquery.MaxLimit)
	sortKey, sortDesc := f.SortKey, f.SortDesc
	if sortKey == "" {
		sortKey, sortDesc = ListSort.Columns[ListSort.Default.Field], ListSort.Default.Desc
	}
	args := db.ListWarrantyClaimsInScopeParams{
		BrandID: c.BrandID, OrganizationIds: c.Filter.OrgIDsArg(), Statuses: f.Statuses,
		OrganizationUuids: f.OrganizationUUIDs,
		CreatedFrom:       tsPtr(f.CreatedFrom), CreatedTo: tsPtr(f.CreatedTo), PageLimit: limit, PageOffset: offset,
		SortKey: sortKey, SortDesc: sortDesc,
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		args.Q = pgtype.Text{String: escapeLike(q), Valid: true}
		args.QExact = pgtype.Text{String: q, Valid: true}
	}
	if f.WarrantyID != uuid.Nil {
		w, err := s.q.GetWarrantyByUUID(ctx, db.GetWarrantyByUUIDParams{Uuid: f.WarrantyID, BrandID: c.BrandID})
		if err != nil {
			return args, mapDB(err)
		}
		args.WarrantyID = int8(w.ID)
	}
	if f.ServiceID != uuid.Nil {
		svc, err := s.q.GetServiceByUUID(ctx, db.GetServiceByUUIDParams{Uuid: f.ServiceID, BrandID: c.BrandID})
		if err != nil {
			return args, mapDB(err)
		}
		args.ServiceID = int8(svc.ID)
	}
	if f.VehicleID != uuid.Nil {
		veh, err := s.q.GetVehicleByUUID(ctx, f.VehicleID)
		if err != nil {
			return args, mapDB(err)
		}
		if veh.BrandID != c.BrandID {
			return args, ErrNotFound
		}
		args.VehicleID = int8(veh.ID)
	}
	return args, nil
}

func (s *Service) view(ctx context.Context, row db.WarrantyClaim, detail bool) (model.ClaimView, error) {
	var cc model.CoverageCheck
	if len(row.CoverageCheck) > 0 {
		_ = json.Unmarshal(row.CoverageCheck, &cc)
	}
	v := model.ClaimView{
		UUID: row.Uuid, ClaimNo: row.ClaimNo, Status: row.Status, Description: row.Description,
		CoverageCheck: cc, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	w, err := s.q.GetWarrantyClaimOpenContext(ctx, db.GetWarrantyClaimOpenContextParams{ID: row.ID, BrandID: row.BrandID})
	if err == nil {
		v.WarrantyUUID = w.WarrantyUuid
	}
	v.RejectionReason = textPtr(row.RejectionReason)
	v.AIDamageType = textPtr(row.AiDamageType)
	v.AISummary = textPtr(row.AiSummary)
	v.AITriagedAt = tsTextPtr(row.AiTriagedAt)
	if row.AiConfidence.Valid {
		v.AIConfidence = stringPtr(row.AiConfidence.Int.String())
	}
	if !detail {
		return v, nil
	}
	parts, err := s.q.ListWarrantyClaimParts(ctx, row.ID)
	if err != nil {
		return v, err
	}
	v.Parts = make([]model.PartView, 0, len(parts))
	for _, p := range parts {
		v.Parts = append(v.Parts, model.PartView{UUID: p.Uuid, PartKey: p.PartKey, Note: p.Note})
	}
	photos, err := s.q.ListWarrantyClaimPhotos(ctx, row.ID)
	if err != nil {
		return v, err
	}
	v.Photos = make([]model.PhotoView, 0, len(photos))
	for _, p := range photos {
		v.Photos = append(v.Photos, photoView(p))
	}
	return v, nil
}

func readPhoto(in PhotoInput) ([]byte, string, string, string, error) {
	if in.Body == nil {
		return nil, "", "", "", invalid("file", "file is required")
	}
	if in.Size > MaxPhotoBytes {
		return nil, "", "", "", invalid("file", "file is too large")
	}
	data, err := io.ReadAll(io.LimitReader(in.Body, MaxPhotoBytes+1))
	if err != nil {
		return nil, "", "", "", err
	}
	if len(data) == 0 || len(data) > MaxPhotoBytes {
		return nil, "", "", "", invalid("file", "file is too large")
	}
	mimeType := http.DetectContentType(data)
	ext := ""
	switch mimeType {
	case "image/jpeg":
		ext = "jpg"
	case "image/png":
		ext = "png"
	case "image/webp":
		ext = "webp"
	default:
		return nil, "", "", "", invalid("file", "unsupported image type")
	}
	if byName := strings.TrimPrefix(strings.ToLower(filepath.Ext(in.Filename)), "."); byName != "" {
		if mt := mime.TypeByExtension("." + byName); mt != "" && strings.HasPrefix(mt, "image/") {
			_ = mt
		}
	}
	sum := sha256.Sum256(data)
	return data, mimeType, ext, hex.EncodeToString(sum[:]), nil
}

func photoView(p db.WarrantyClaimPhoto) model.PhotoView {
	return model.PhotoView{UUID: p.Uuid, MimeType: p.MimeType, SizeBytes: p.SizeBytes, CreatedAt: p.CreatedAt.Time}
}

func portalView(r db.WarrantyClaim) model.PortalClaimView {
	return model.PortalClaimView{UUID: r.Uuid, Status: r.Status, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
}

func mapCreateErr(err error) error {
	if isUnique(err, "uq_warranty_claims_live_warranty") {
		return ErrConflict
	}
	return mapDB(err)
}

func mapDB(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func isUnique(err error, name string) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505" && pg.ConstraintName == name
}

func validPartKey(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return true
}

func countArgs(a db.ListWarrantyClaimsInScopeParams) db.CountWarrantyClaimsInScopeParams {
	return db.CountWarrantyClaimsInScopeParams{
		BrandID: a.BrandID, OrganizationIds: a.OrganizationIds, Statuses: a.Statuses,
		WarrantyID: a.WarrantyID, ServiceID: a.ServiceID, VehicleID: a.VehicleID,
		CustomerUserID: a.CustomerUserID, CreatedFrom: a.CreatedFrom, CreatedTo: a.CreatedTo, Q: a.Q,
		OrganizationUuids: a.OrganizationUuids, QExact: a.QExact,
	}
}

func int8(v int64) pgtype.Int8 {
	if v <= 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: v, Valid: true}
}

func text(v string) pgtype.Text {
	if strings.TrimSpace(v) == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func tsTextPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

func stringPtr(s string) *string { return &s }

func tsPtr(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func uniquePositive(ids []int64) []int64 {
	out := []int64{}
	for _, id := range ids {
		if id <= 0 || slices.Contains(out, id) {
			continue
		}
		out = append(out, id)
	}
	return out
}

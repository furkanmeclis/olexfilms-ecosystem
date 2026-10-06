// Package usecase implements contract template CRUD and rendering.
package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"sort"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts/repository"
	docmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/otp"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	platstorage "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrNotFound          = errors.New("contract not found")
	ErrInvalidRequest    = errors.New("invalid contract request")
	ErrInUse             = errors.New("contract template is in use")
	ErrBusinessRule      = errors.New("contract business rule")
	ErrAlreadySigned     = errors.New("contract signer already signed")
	ErrWindowExpired     = errors.New("contract sign window expired")
	ErrStorageRequired   = errors.New("contract storage is not configured")
	ErrOTPRequired       = errors.New("contract otp service is not configured")
	ErrOutboxRequired    = errors.New("contract outbox is not configured")
	ErrInvalidOTP        = errors.New("contract otp is invalid")
	ErrUnsupportedStatus = errors.New("contract service status does not allow signing")
)

const (
	CodeContractSignWindowExpired = "CONTRACT_SIGN_WINDOW_EXPIRED"
	CodeContractServiceStatus     = "CONTRACT_SERVICE_STATUS_INVALID"
	CodeContractAlreadySigned     = "CONTRACT_ALREADY_SIGNED"

	MaxSignatureBytes = 1 << 20
	MaxMediaBytes     = 12 << 20
	signWindow        = 30 * time.Minute
)

// UnknownVariablesError lists placeholders that are not allowed.
type UnknownVariablesError struct{ Keys []string }

func (e *UnknownVariablesError) Error() string {
	return "unknown contract variables: " + strings.Join(e.Keys, ", ")
}

const MaxTemplateBytes = 512 * 1024

// Caller is the active organization context.
type Caller struct {
	UserID         int64
	OrganizationID int64
	BrandID        int64
	Filter         scopefilter.Filter
}

// Input creates or patches template metadata.
type Input struct {
	Name              string
	Kind              string
	IsDefault         *bool
	OTPRequired       *bool
	SignatureRequired *bool
	IsActive          *bool
}

// LocaleInput creates or replaces a language version.
type LocaleInput struct {
	Locale      string
	LexicalJSON json.RawMessage
	HTML        string
}

// ListFilter filters templates (TEC-369): Kinds is any of; Active and
// IsDefault are true / false / nil (no filter).
type ListFilter struct {
	Kinds     []string
	Active    *bool
	IsDefault *bool
}

// Service is the contract template use case.
type Service struct {
	repo    *repository.Store
	otp     OTPService
	storage platstorage.Driver
	pdf     PDFRenderer
	out     outbox.Enqueuer
	now     func() time.Time
}

// New creates a service.
func New(repo *repository.Store, opts ...Option) *Service {
	s := &Service{repo: repo, now: time.Now}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Option wires optional runtime dependencies for the signing flow.
type Option func(*Service)

func WithOTP(svc OTPService) Option { return func(s *Service) { s.otp = svc } }
func WithStorage(store platstorage.Driver) Option {
	return func(s *Service) { s.storage = store }
}
func WithPDFRenderer(pdf PDFRenderer) Option { return func(s *Service) { s.pdf = pdf } }
func WithOutbox(out outbox.Enqueuer) Option  { return func(s *Service) { s.out = out } }
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

// OTPService is the phone OTP port used by contract signing.
type OTPService interface {
	Request(ctx context.Context, in otp.RequestInput) (otp.RequestResult, error)
	Verify(ctx context.Context, in otp.VerifyInput) (otp.Verified, error)
}

// PDFRenderer converts an executed contract HTML snapshot to a PDF.
type PDFRenderer interface {
	Convert(ctx context.Context, req pdfrender.Request) ([]byte, error)
}

// ContractDocumentLoader exposes executed contracts to the shared documents
// template engine under kind=contract.
func (s *Service) ContractDocumentLoader() docmodel.SourceLoader {
	return contractDocumentLoader{s: s}
}

type CreateFromServiceInput struct {
	TemplateUUID uuid.UUID
	Locale       string
}

type OTPInput struct {
	IP        string
	UserAgent string
	Locale    string
}

type SignatureInput struct {
	Code      string
	PNGBase64 string
	IP        string
	UserAgent string
}

type MediaInput struct {
	Body        io.Reader
	Size        int64
	Filename    string
	Title       string
	ContentType string
}

// Variables returns the fixed allow-list.
func (s *Service) Variables() []model.Variable { return variables }

// List returns templates for the active brand.
func (s *Service) List(ctx context.Context, c Caller, f ListFilter) ([]model.Template, error) {
	kinds := make([]string, 0, len(f.Kinds))
	for _, raw := range f.Kinds {
		k, err := normalizeKind(raw)
		if err != nil {
			return nil, err
		}
		kinds = append(kinds, k)
	}
	rows, err := s.repo.Queries().ListContractTemplates(ctx, db.ListContractTemplatesParams{
		BrandID: c.BrandID, Kinds: kinds, IsActive: optBool(f.Active), IsDefault: optBool(f.IsDefault),
	})
	if err != nil {
		return nil, err
	}
	out := make([]model.Template, 0, len(rows))
	for _, r := range rows {
		t, err := s.view(ctx, r, true)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// Get returns one template.
func (s *Service) Get(ctx context.Context, c Caller, id uuid.UUID) (model.Template, error) {
	tpl, err := s.repo.Queries().GetContractTemplateByUUID(ctx, db.GetContractTemplateByUUIDParams{Uuid: id, BrandID: c.BrandID})
	if err != nil {
		return model.Template{}, notFound(err)
	}
	return s.view(ctx, tpl, true)
}

// Create stores template metadata. When input asks for default, the old
// default of the same kind is cleared in the same transaction.
func (s *Service) Create(ctx context.Context, c Caller, in Input) (model.Template, error) {
	name, err := normalizeName(in.Name)
	if err != nil {
		return model.Template{}, err
	}
	kind, err := normalizeKind(in.Kind)
	if err != nil {
		return model.Template{}, err
	}
	otp, sig, active := true, true, true
	if in.OTPRequired != nil {
		otp = *in.OTPRequired
	}
	if in.SignatureRequired != nil {
		sig = *in.SignatureRequired
	}
	if in.IsActive != nil {
		active = *in.IsActive
	}
	wantDefault := in.IsDefault != nil && *in.IsDefault
	if wantDefault {
		active = true
	}
	tx, qtx, err := s.repo.Tx(ctx)
	if err != nil {
		return model.Template{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	row, err := qtx.CreateContractTemplate(ctx, db.CreateContractTemplateParams{
		OrganizationID: c.OrganizationID, BrandID: c.BrandID, Name: name, Kind: kind,
		IsDefault: false, OtpRequired: otp, SignatureRequired: sig, IsActive: active,
		CreatedByUserID: int8(c.UserID),
	})
	if err != nil {
		return model.Template{}, err
	}
	if wantDefault {
		if err := qtx.ClearDefaultContractTemplate(ctx, db.ClearDefaultContractTemplateParams{BrandID: c.BrandID, Kind: kind, KeepID: row.ID}); err != nil {
			return model.Template{}, err
		}
		row, err = qtx.UpdateContractTemplate(ctx, db.UpdateContractTemplateParams{
			ID: row.ID, BrandID: c.BrandID, Name: name, IsDefault: true, OtpRequired: otp,
			SignatureRequired: sig, IsActive: true, UpdatedByUserID: int8(c.UserID),
		})
		if err != nil {
			return model.Template{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Template{}, err
	}
	return s.view(ctx, row, true)
}

// Update patches template metadata.
func (s *Service) Update(ctx context.Context, c Caller, id uuid.UUID, in Input) (model.Template, error) {
	current, err := s.repo.Queries().GetContractTemplateByUUID(ctx, db.GetContractTemplateByUUIDParams{Uuid: id, BrandID: c.BrandID})
	if err != nil {
		return model.Template{}, notFound(err)
	}
	name := current.Name
	if strings.TrimSpace(in.Name) != "" {
		name, err = normalizeName(in.Name)
		if err != nil {
			return model.Template{}, err
		}
	}
	otp, sig, active, def := current.OtpRequired, current.SignatureRequired, current.IsActive, current.IsDefault
	if in.OTPRequired != nil {
		otp = *in.OTPRequired
	}
	if in.SignatureRequired != nil {
		sig = *in.SignatureRequired
	}
	if in.IsActive != nil {
		active = *in.IsActive
	}
	if in.IsDefault != nil {
		def = *in.IsDefault
	}
	if def {
		active = true
	}
	tx, qtx, err := s.repo.Tx(ctx)
	if err != nil {
		return model.Template{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if def {
		if err := qtx.ClearDefaultContractTemplate(ctx, db.ClearDefaultContractTemplateParams{
			BrandID: c.BrandID, Kind: current.Kind, KeepID: current.ID,
		}); err != nil {
			return model.Template{}, err
		}
	}
	row, err := qtx.UpdateContractTemplate(ctx, db.UpdateContractTemplateParams{
		ID: current.ID, BrandID: c.BrandID, Name: name, IsDefault: def, OtpRequired: otp,
		SignatureRequired: sig, IsActive: active, UpdatedByUserID: int8(c.UserID),
	})
	if err != nil {
		return model.Template{}, notFound(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Template{}, err
	}
	return s.view(ctx, row, true)
}

// Delete removes an unused template; if it is frozen by instances, it is
// deactivated instead.
func (s *Service) Delete(ctx context.Context, c Caller, id uuid.UUID) (model.Template, bool, error) {
	current, err := s.repo.Queries().GetContractTemplateByUUID(ctx, db.GetContractTemplateByUUIDParams{Uuid: id, BrandID: c.BrandID})
	if err != nil {
		return model.Template{}, false, notFound(err)
	}
	tx, qtx, err := s.repo.Tx(ctx)
	if err != nil {
		return model.Template{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	n, err := repository.CountTemplateInstancesTx(ctx, tx, current.ID)
	if err != nil {
		return model.Template{}, false, err
	}
	if n == 0 {
		aff, err := qtx.DeleteContractTemplate(ctx, db.DeleteContractTemplateParams{ID: current.ID, BrandID: c.BrandID})
		if err != nil {
			return model.Template{}, false, err
		}
		if aff == 0 {
			return model.Template{}, false, ErrNotFound
		}
		if err := tx.Commit(ctx); err != nil {
			return model.Template{}, false, err
		}
		return model.Template{}, true, nil
	}
	row, err := qtx.UpdateContractTemplate(ctx, db.UpdateContractTemplateParams{
		ID: current.ID, BrandID: c.BrandID, Name: current.Name, IsDefault: false,
		OtpRequired: current.OtpRequired, SignatureRequired: current.SignatureRequired,
		IsActive: false, UpdatedByUserID: int8(c.UserID),
	})
	if err != nil {
		return model.Template{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Template{}, false, err
	}
	v, err := s.view(ctx, row, true)
	return v, false, err
}

// SetDefault makes one active template the single default of its kind.
func (s *Service) SetDefault(ctx context.Context, c Caller, id uuid.UUID) (model.Template, error) {
	current, err := s.repo.Queries().GetContractTemplateByUUID(ctx, db.GetContractTemplateByUUIDParams{Uuid: id, BrandID: c.BrandID})
	if err != nil {
		return model.Template{}, notFound(err)
	}
	tx, qtx, err := s.repo.Tx(ctx)
	if err != nil {
		return model.Template{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := qtx.ClearDefaultContractTemplate(ctx, db.ClearDefaultContractTemplateParams{
		BrandID: c.BrandID, Kind: current.Kind, KeepID: current.ID,
	}); err != nil {
		return model.Template{}, err
	}
	row, err := qtx.UpdateContractTemplate(ctx, db.UpdateContractTemplateParams{
		ID: current.ID, BrandID: c.BrandID, Name: current.Name, IsDefault: true,
		OtpRequired: current.OtpRequired, SignatureRequired: current.SignatureRequired,
		IsActive: true, UpdatedByUserID: int8(c.UserID),
	})
	if err != nil {
		return model.Template{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Template{}, err
	}
	return s.view(ctx, row, true)
}

// PutLocale sanitizes and stores one locale version, bumping version on each write.
func (s *Service) PutLocale(ctx context.Context, c Caller, id uuid.UUID, in LocaleInput) (model.TemplateLocale, error) {
	tpl, err := s.repo.Queries().GetContractTemplateByUUID(ctx, db.GetContractTemplateByUUIDParams{Uuid: id, BrandID: c.BrandID})
	if err != nil {
		return model.TemplateLocale{}, notFound(err)
	}
	loc := NormalizeLocale(in.Locale)
	if loc == "" {
		return model.TemplateLocale{}, fmt.Errorf("%w: unsupported locale", ErrInvalidRequest)
	}
	html, err := PrepareHTML(in.HTML)
	if err != nil {
		return model.TemplateLocale{}, err
	}
	lex, err := lexicalBytes(in.LexicalJSON)
	if err != nil {
		return model.TemplateLocale{}, err
	}
	row, err := s.repo.Queries().UpsertContractTemplateLocale(ctx, db.UpsertContractTemplateLocaleParams{
		TemplateID: tpl.ID, OrganizationID: tpl.OrganizationID, BrandID: tpl.BrandID,
		Locale: loc, LexicalJson: lex,
		Html: html, UpdatedByUserID: int8(c.UserID),
	})
	if err != nil {
		return model.TemplateLocale{}, err
	}
	return localeView(row), nil
}

// CreateForService freezes a rendered contract snapshot for a draft/pending service.
func (s *Service) CreateForService(ctx context.Context, c Caller, serviceUUID uuid.UUID, in CreateFromServiceInput) (model.Contract, error) {
	f := c.Filter
	row, err := s.repo.Queries().GetServiceForContractByUUID(ctx, db.GetServiceForContractByUUIDParams{
		Uuid: serviceUUID, BrandID: brandArg(c, f), OrgIds: orgIDsArg(c, f),
	})
	if err != nil {
		return model.Contract{}, notFound(err)
	}
	if f.UserOnly() && (!row.CreatedByUserID.Valid || row.CreatedByUserID.Int64 != f.UserID) {
		return model.Contract{}, ErrNotFound
	}
	if row.Status != "draft" && row.Status != "pending" {
		return model.Contract{}, fmt.Errorf("%w: service status %s", ErrUnsupportedStatus, row.Status)
	}
	tpl, err := s.resolveTemplateForInstance(ctx, c.BrandID, in.TemplateUUID)
	if err != nil {
		return model.Contract{}, err
	}
	loc, err := s.resolveLocale(ctx, tpl.ID, in.Locale)
	if err != nil {
		return model.Contract{}, err
	}
	rendered := pdfrender.Fill(pdfrender.SanitizeHTML(loc.Html), serviceValues(row), nil)
	sum := sha256Hex([]byte(rendered))

	tx, qtx, err := s.repo.Tx(ctx)
	if err != nil {
		return model.Contract{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	no, err := qtx.NextContractNo(ctx, row.OrganizationID)
	if err != nil {
		return model.Contract{}, err
	}
	inst, err := qtx.CreateContractInstance(ctx, db.CreateContractInstanceParams{
		OrganizationID: row.OrganizationID, BrandID: row.BrandID, ContractNo: no,
		SubjectType: "service", SubjectID: row.ID, TemplateID: tpl.ID, Kind: tpl.Kind,
		Locale: loc.Locale, TemplateVersion: loc.Version, OtpRequired: tpl.OtpRequired,
		SignatureRequired: tpl.SignatureRequired, Status: model.StatusPending,
		RenderedHtml: pgText(rendered), ContentSha256: pgText(sum), CreatedByUserID: int8(c.UserID),
	})
	if err != nil {
		return model.Contract{}, err
	}
	if _, err := qtx.UpsertContractSigner(ctx, db.UpsertContractSignerParams{
		InstanceID: inst.ID, OrganizationID: inst.OrganizationID, BrandID: inst.BrandID,
		Role: model.SignerCustomer, UserID: pgtype.Int8{Int64: row.CustomerUserID, Valid: true},
		Name: signerNameString(row.CustomerName, row.CustomerSurname, "Customer"), PhoneE164: row.CustomerPhone,
	}); err != nil {
		return model.Contract{}, err
	}
	if _, err := qtx.UpsertContractSigner(ctx, db.UpsertContractSignerParams{
		InstanceID: inst.ID, OrganizationID: inst.OrganizationID, BrandID: inst.BrandID,
		Role: model.SignerStaff, UserID: row.CreatedByUserID,
		Name: signerName(row.StaffName, row.StaffSurname, "Staff"),
	}); err != nil {
		return model.Contract{}, err
	}
	if aff, err := qtx.SetServiceContract(ctx, db.SetServiceContractParams{
		ServiceID: row.ID, ContractID: pgtype.Int8{Int64: inst.ID, Valid: true},
	}); err != nil {
		return model.Contract{}, err
	} else if aff == 0 {
		return model.Contract{}, ErrNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Contract{}, err
	}
	return s.viewContract(ctx, inst)
}

// GetContract returns one contract in the caller's read scope.
func (s *Service) GetContract(ctx context.Context, c Caller, id uuid.UUID) (model.Contract, error) {
	inst, err := s.scopedInstance(ctx, c, id)
	if err != nil {
		return model.Contract{}, err
	}
	return s.viewContract(ctx, inst)
}

// RequestCustomerOTP sends a contract_sign code to the customer signer.
func (s *Service) RequestCustomerOTP(ctx context.Context, c Caller, id uuid.UUID, in OTPInput) (otp.RequestResult, error) {
	if s.otp == nil {
		return otp.RequestResult{}, ErrOTPRequired
	}
	inst, err := s.scopedInstance(ctx, c, id)
	if err != nil {
		return otp.RequestResult{}, err
	}
	if inst.Status == model.StatusExecuted || inst.Status == model.StatusVoided {
		return otp.RequestResult{}, ErrAlreadySigned
	}
	if !inst.OtpRequired {
		return otp.RequestResult{}, nil
	}
	signer, err := s.repo.Queries().GetContractSigner(ctx, db.GetContractSignerParams{InstanceID: inst.ID, Role: model.SignerCustomer})
	if err != nil {
		return otp.RequestResult{}, err
	}
	if signer.SignedAt.Valid {
		return otp.RequestResult{}, ErrAlreadySigned
	}
	if !signer.PhoneE164.Valid {
		return otp.RequestResult{}, fmt.Errorf("%w: customer phone is required", ErrInvalidRequest)
	}
	return s.otp.Request(ctx, otp.RequestInput{
		Phone: signer.PhoneE164.String, Purpose: otp.PurposeContractSign, Locale: in.Locale,
		IP: strings.TrimSpace(in.IP), UserAgent: trimUA(in.UserAgent),
	})
}

// SignCustomer verifies OTP when required and stores customer signature evidence.
func (s *Service) SignCustomer(ctx context.Context, c Caller, id uuid.UUID, in SignatureInput) (model.Contract, error) {
	inst, err := s.scopedInstance(ctx, c, id)
	if err != nil {
		return model.Contract{}, err
	}
	signer, err := s.repo.Queries().GetContractSigner(ctx, db.GetContractSignerParams{InstanceID: inst.ID, Role: model.SignerCustomer})
	if err != nil {
		return model.Contract{}, err
	}
	if signer.SignedAt.Valid {
		return model.Contract{}, ErrAlreadySigned
	}
	if inst.OtpRequired {
		if s.otp == nil {
			return model.Contract{}, ErrOTPRequired
		}
		if !signer.PhoneE164.Valid {
			return model.Contract{}, fmt.Errorf("%w: customer phone is required", ErrInvalidRequest)
		}
		verified, err := s.otp.Verify(ctx, otp.VerifyInput{
			Phone: signer.PhoneE164.String, Purpose: otp.PurposeContractSign, Code: in.Code, IP: strings.TrimSpace(in.IP),
		})
		if err != nil {
			if errors.Is(err, otp.ErrInvalidCode) || errors.Is(err, otp.ErrTooManyAttempts) {
				return model.Contract{}, ErrInvalidOTP
			}
			return model.Contract{}, err
		}
		code, err := s.repo.Queries().GetOTPByUUID(ctx, verified.ID)
		if err != nil {
			return model.Contract{}, err
		}
		if s.now().UTC().After(code.CreatedAt.Time.Add(signWindow)) {
			return model.Contract{}, ErrWindowExpired
		}
		signer, err = s.repo.Queries().SetContractSignerOTPByUUID(ctx, db.SetContractSignerOTPByUUIDParams{
			SignerID: signer.ID, OtpUuid: verified.ID,
		})
		if err != nil {
			return model.Contract{}, err
		}
	}
	return s.sign(ctx, c, inst, signer, in.PNGBase64, in.IP, in.UserAgent)
}

// SignStaff stores the authenticated staff user's signature evidence.
func (s *Service) SignStaff(ctx context.Context, c Caller, id uuid.UUID, in SignatureInput) (model.Contract, error) {
	inst, err := s.scopedInstance(ctx, c, id)
	if err != nil {
		return model.Contract{}, err
	}
	signer, err := s.repo.Queries().GetContractSigner(ctx, db.GetContractSignerParams{InstanceID: inst.ID, Role: model.SignerStaff})
	if err != nil {
		return model.Contract{}, err
	}
	if signer.SignedAt.Valid {
		return model.Contract{}, ErrAlreadySigned
	}
	if c.UserID > 0 && signer.UserID.Valid && signer.UserID.Int64 != c.UserID {
		return model.Contract{}, ErrNotFound
	}
	return s.sign(ctx, c, inst, signer, in.PNGBase64, in.IP, in.UserAgent)
}

// AddMedia attaches an image to an open contract.
func (s *Service) AddMedia(ctx context.Context, c Caller, id uuid.UUID, in MediaInput) (model.Media, error) {
	if s.storage == nil {
		return model.Media{}, ErrStorageRequired
	}
	inst, err := s.scopedInstance(ctx, c, id)
	if err != nil {
		return model.Media{}, err
	}
	if inst.Status == model.StatusExecuted || inst.Status == model.StatusVoided {
		return model.Media{}, ErrAlreadySigned
	}
	body, mime, sum, err := readSniffed(in.Body, in.Size, MaxMediaBytes, map[string]bool{
		"image/jpeg": true, "image/png": true, "image/webp": true,
	})
	if err != nil {
		return model.Media{}, err
	}
	key := fmt.Sprintf("contracts/%d/%s", inst.ID, uuid.NewString())
	if err := s.storage.Upload(ctx, platstorage.File{
		Body: bytes.NewReader(body), Size: int64(len(body)), ContentType: mime, Filename: in.Filename,
	}, key); err != nil {
		return model.Media{}, err
	}
	row, err := s.repo.Queries().InsertContractMedia(ctx, db.InsertContractMediaParams{
		InstanceID: inst.ID, OrganizationID: inst.OrganizationID, BrandID: inst.BrandID,
		StorageKey: key, MimeType: mime, SizeBytes: int64(len(body)), Sha256: sum,
		Title: pgText(strings.TrimSpace(in.Title)), SortOrder: 0, UploadedByUserID: int8(c.UserID),
	})
	if err != nil {
		return model.Media{}, err
	}
	return mediaView(row), nil
}

// DeleteMedia removes an image while the contract is still open.
func (s *Service) DeleteMedia(ctx context.Context, c Caller, id, mediaID uuid.UUID) error {
	inst, err := s.scopedInstance(ctx, c, id)
	if err != nil {
		return err
	}
	if inst.Status == model.StatusExecuted || inst.Status == model.StatusVoided {
		return ErrAlreadySigned
	}
	row, err := s.repo.Queries().GetContractMediaByUUID(ctx, db.GetContractMediaByUUIDParams{Uuid: mediaID, InstanceID: inst.ID})
	if err != nil {
		return notFound(err)
	}
	if aff, err := s.repo.Queries().DeleteContractMedia(ctx, db.DeleteContractMediaParams{Uuid: mediaID, InstanceID: inst.ID}); err != nil {
		return err
	} else if aff == 0 {
		return ErrNotFound
	}
	if s.storage != nil {
		_ = s.storage.Delete(ctx, row.StorageKey)
	}
	return nil
}

// Void marks a contract voided with a required reason.
func (s *Service) Void(ctx context.Context, c Caller, id uuid.UUID, reason string) (model.Contract, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return model.Contract{}, fmt.Errorf("%w: reason is required", ErrInvalidRequest)
	}
	inst, err := s.scopedInstance(ctx, c, id)
	if err != nil {
		return model.Contract{}, err
	}
	row, err := s.repo.Queries().VoidContractInstance(ctx, db.VoidContractInstanceParams{
		ID: inst.ID, VoidReason: reason, VoidedByUserID: int8(c.UserID),
	})
	if err != nil {
		return model.Contract{}, err
	}
	return s.viewContract(ctx, row)
}

// Render fills the selected locale (requested, then tr, then en) with escaped values.
func (s *Service) Render(ctx context.Context, c Caller, in model.RenderInput) (model.Rendered, error) {
	tpl, err := s.repo.Queries().GetContractTemplateByUUID(ctx, db.GetContractTemplateByUUIDParams{Uuid: in.TemplateUUID, BrandID: c.BrandID})
	if err != nil {
		return model.Rendered{}, notFound(err)
	}
	loc, err := s.resolveLocale(ctx, tpl.ID, in.Locale)
	if err != nil {
		return model.Rendered{}, err
	}
	return model.Rendered{
		HTML:   pdfrender.Fill(pdfrender.SanitizeHTML(loc.Html), in.Values, nil),
		Locale: loc.Locale, Version: loc.Version,
	}, nil
}

func (s *Service) resolveLocale(ctx context.Context, templateID int64, requested string) (db.ContractTemplateLocale, error) {
	candidates := []string{}
	if loc := NormalizeLocale(requested); loc != "" {
		candidates = append(candidates, loc)
	}
	candidates = append(candidates, "tr", "en")
	seen := map[string]bool{}
	for _, loc := range candidates {
		if seen[loc] {
			continue
		}
		seen[loc] = true
		row, err := s.repo.Queries().GetContractTemplateLocale(ctx, db.GetContractTemplateLocaleParams{TemplateID: templateID, Locale: loc})
		if err == nil {
			return row, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return db.ContractTemplateLocale{}, err
		}
	}
	return db.ContractTemplateLocale{}, ErrNotFound
}

func (s *Service) resolveTemplateForInstance(ctx context.Context, brandID int64, templateID uuid.UUID) (db.ContractTemplate, error) {
	if templateID != uuid.Nil {
		tpl, err := s.repo.Queries().GetContractTemplateByUUID(ctx, db.GetContractTemplateByUUIDParams{Uuid: templateID, BrandID: brandID})
		if err != nil {
			return db.ContractTemplate{}, notFound(err)
		}
		if !tpl.IsActive {
			return db.ContractTemplate{}, fmt.Errorf("%w: template is inactive", ErrInvalidRequest)
		}
		return tpl, nil
	}
	tpl, err := s.repo.Queries().GetDefaultContractTemplate(ctx, db.GetDefaultContractTemplateParams{BrandID: brandID, Kind: model.KindVehicleIntake})
	if err != nil {
		return db.ContractTemplate{}, notFound(err)
	}
	if !tpl.IsActive {
		return db.ContractTemplate{}, fmt.Errorf("%w: default template is inactive", ErrInvalidRequest)
	}
	return tpl, nil
}

func (s *Service) scopedInstance(ctx context.Context, c Caller, id uuid.UUID) (db.ContractInstance, error) {
	f := c.Filter
	row, err := s.repo.Queries().GetContractInstanceByUUIDScoped(ctx, db.GetContractInstanceByUUIDScopedParams{
		Uuid: id, BrandID: brandArg(c, f), OrgIds: orgIDsArg(c, f),
	})
	if err != nil {
		return db.ContractInstance{}, notFound(err)
	}
	if f.UserOnly() {
		switch row.SubjectType {
		case "service":
			svc, err := s.repo.Queries().GetServiceForContractByID(ctx, row.SubjectID)
			if err != nil {
				return db.ContractInstance{}, err
			}
			if !svc.CreatedByUserID.Valid || svc.CreatedByUserID.Int64 != f.UserID {
				return db.ContractInstance{}, ErrNotFound
			}
		case "service_subscription":
			sub, err := s.repo.Queries().GetServiceSubscriptionForContractByID(ctx, row.SubjectID)
			if err != nil {
				return db.ContractInstance{}, err
			}
			if !sub.AssignedByUserID.Valid || sub.AssignedByUserID.Int64 != f.UserID {
				return db.ContractInstance{}, ErrNotFound
			}
		default:
			return db.ContractInstance{}, ErrNotFound
		}
	}
	return row, nil
}

func (s *Service) sign(ctx context.Context, c Caller, inst db.ContractInstance, signer db.ContractSigner, png64, ip, ua string) (model.Contract, error) {
	var body []byte
	var sum, key string
	if strings.TrimSpace(png64) != "" {
		if s.storage == nil {
			return model.Contract{}, ErrStorageRequired
		}
		var err error
		body, sum, err = decodePNGBase64(png64)
		if err != nil {
			return model.Contract{}, err
		}
		key = fmt.Sprintf("contracts/%d/signatures/%s.png", inst.ID, uuid.NewString())
		if err := s.storage.Upload(ctx, platstorage.File{
			Body: bytes.NewReader(body), Size: int64(len(body)), ContentType: "image/png", Filename: signer.Role + ".png",
		}, key); err != nil {
			return model.Contract{}, err
		}
	} else if inst.SignatureRequired {
		return model.Contract{}, fmt.Errorf("%w: signature_png is required", ErrInvalidRequest)
	}

	tx, qtx, err := s.repo.Tx(ctx)
	if err != nil {
		return model.Contract{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	locked, err := qtx.GetContractInstanceForUpdate(ctx, inst.ID)
	if err != nil {
		return model.Contract{}, err
	}
	if locked.Status == model.StatusExecuted || locked.Status == model.StatusVoided {
		return model.Contract{}, ErrAlreadySigned
	}
	current, err := qtx.GetContractSigner(ctx, db.GetContractSignerParams{InstanceID: locked.ID, Role: signer.Role})
	if err != nil {
		return model.Contract{}, err
	}
	if current.SignedAt.Valid {
		return model.Contract{}, ErrAlreadySigned
	}
	if key != "" {
		if _, err := qtx.InsertContractSignature(ctx, db.InsertContractSignatureParams{
			SignerID: current.ID, InstanceID: locked.ID, OrganizationID: locked.OrganizationID, BrandID: locked.BrandID,
			StorageKey: key, Sha256: sum, IpAddress: netipOrNil(ip), UserAgent: pgText(trimUA(ua)),
		}); err != nil {
			return model.Contract{}, err
		}
	}
	if _, err := qtx.MarkContractSignerSigned(ctx, current.ID); err != nil {
		return model.Contract{}, err
	}
	after, err := qtx.ListContractSigners(ctx, locked.ID)
	if err != nil {
		return model.Contract{}, err
	}
	allSigned := len(after) >= 2
	for _, sg := range after {
		allSigned = allSigned && sg.SignedAt.Valid
	}
	if allSigned {
		locked, err = qtx.ExecuteContractInstance(ctx, db.ExecuteContractInstanceParams{
			ID: locked.ID, RenderedHtml: locked.RenderedHtml.String, ContentSha256: locked.ContentSha256.String,
		})
		if err != nil {
			return model.Contract{}, err
		}
		if s.out == nil {
			return model.Contract{}, ErrOutboxRequired
		}
		id, uid := locked.ID, locked.Uuid
		ev := events.New(events.ContractExecuted).WithTenant(locked.OrganizationID).
			WithEntity("contract", &id, &uid).
			WithPayload(map[string]any{
				"contract_uuid":   locked.Uuid.String(),
				"contract_no":     locked.ContractNo,
				"subject_type":    locked.SubjectType,
				"subject_id":      locked.SubjectID,
				"organization_id": locked.OrganizationID,
				"brand_id":        locked.BrandID,
			})
		if c.UserID > 0 {
			ev = ev.WithActor(c.UserID)
		}
		if err := s.out.Enqueue(ctx, tx, ev); err != nil {
			return model.Contract{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Contract{}, err
	}
	return s.viewContract(ctx, locked)
}

func (s *Service) viewContract(ctx context.Context, row db.ContractInstance) (model.Contract, error) {
	signers, err := s.repo.Queries().ListContractSigners(ctx, row.ID)
	if err != nil {
		return model.Contract{}, err
	}
	media, err := s.repo.Queries().ListContractMedia(ctx, row.ID)
	if err != nil {
		return model.Contract{}, err
	}
	v := model.Contract{
		UUID: row.Uuid, ContractNo: row.ContractNo, SubjectType: row.SubjectType, SubjectID: row.SubjectID,
		Kind: row.Kind, Locale: row.Locale, TemplateVersion: row.TemplateVersion,
		OTPRequired: row.OtpRequired, SignatureRequired: row.SignatureRequired, Status: row.Status,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	if row.RenderedHtml.Valid {
		v.RenderedHTML = row.RenderedHtml.String
	}
	if row.ContentSha256.Valid {
		v.ContentSHA256 = row.ContentSha256.String
	}
	if row.ExecutedAt.Valid {
		t := row.ExecutedAt.Time
		v.ExecutedAt = &t
	}
	if row.VoidedAt.Valid {
		t := row.VoidedAt.Time
		v.VoidedAt = &t
	}
	if row.VoidReason.Valid {
		v.VoidReason = row.VoidReason.String
	}
	v.Signers = make([]model.Signer, 0, len(signers))
	for _, sg := range signers {
		item := signerView(sg)
		if sig, err := s.repo.Queries().GetLatestContractSignature(ctx, sg.ID); err == nil {
			item.Signature = ptrSignature(signatureView(sig))
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return model.Contract{}, err
		}
		v.Signers = append(v.Signers, item)
	}
	v.Media = make([]model.Media, 0, len(media))
	for _, m := range media {
		v.Media = append(v.Media, mediaView(m))
	}
	return v, nil
}

func brandArg(c Caller, f scopefilter.Filter) pgtype.Int8 {
	if f.Scope == rbac.ScopeAll {
		return pgtype.Int8{}
	}
	if f.BrandID != 0 {
		return pgtype.Int8{Int64: f.BrandID, Valid: true}
	}
	if c.BrandID != 0 {
		return pgtype.Int8{Int64: c.BrandID, Valid: true}
	}
	return pgtype.Int8{}
}

func orgIDsArg(c Caller, f scopefilter.Filter) []int64 {
	if f.Scope != "" {
		return f.OrgIDsArg()
	}
	if c.OrganizationID != 0 {
		return []int64{c.OrganizationID}
	}
	return nil
}

func serviceValues(row db.GetServiceForContractByUUIDRow) map[string]string {
	vehicle := strings.TrimSpace(strings.Join([]string{
		row.CarBrandName.String, row.CarModelName.String, modelYearString(row.ModelYear),
	}, " "))
	return map[string]string{
		"customer_name":  signerNameString(row.CustomerName, row.CustomerSurname, ""),
		"customer_phone": row.CustomerPhone.String,
		"customer_email": row.CustomerEmail.String,
		"plate":          row.Plate.String,
		"vin":            row.Vin.String,
		"vehicle_label":  vehicle,
		"service_no":     row.ServiceNo,
		"package":        row.Package.String,
		"org_name":       row.OrganizationName,
		"org_phone":      row.OrganizationPhone,
		"org_email":      row.OrganizationEmail,
		"org_address":    row.OrganizationAddress,
		"staff_name":     signerName(row.StaffName, row.StaffSurname, ""),
		"today":          time.Now().Format("2006-01-02"),
	}
}

func signerName(name, surname pgtype.Text, fallback string) string {
	full := strings.TrimSpace(strings.TrimSpace(name.String) + " " + strings.TrimSpace(surname.String))
	if full == "" {
		return fallback
	}
	return full
}

func signerNameString(name, surname, fallback string) string {
	full := strings.TrimSpace(strings.TrimSpace(name) + " " + strings.TrimSpace(surname))
	if full == "" {
		return fallback
	}
	return full
}

func modelYearString(y pgtype.Int2) string {
	if !y.Valid {
		return ""
	}
	return fmt.Sprint(y.Int16)
}

func netipOrNil(raw string) *netip.Addr {
	addr, err := netip.ParseAddr(strings.TrimSpace(raw))
	if err != nil {
		return nil
	}
	return &addr
}

func decodePNGBase64(raw string) ([]byte, string, error) {
	raw = strings.TrimSpace(raw)
	if i := strings.Index(raw, ","); strings.HasPrefix(raw, "data:") && i >= 0 {
		raw = raw[i+1:]
	}
	dec := base64.NewDecoder(base64.StdEncoding, strings.NewReader(raw))
	body, err := io.ReadAll(io.LimitReader(dec, MaxSignatureBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("%w: signature_png must be base64", ErrInvalidRequest)
	}
	if len(body) == 0 || len(body) > MaxSignatureBytes {
		return nil, "", fmt.Errorf("%w: signature_png exceeds 1 MB", ErrInvalidRequest)
	}
	if http.DetectContentType(body) != "image/png" {
		return nil, "", fmt.Errorf("%w: signature_png must be PNG", ErrInvalidRequest)
	}
	return body, sha256Hex(body), nil
}

func readSniffed(r io.Reader, declared, max int64, allowed map[string]bool) ([]byte, string, string, error) {
	if r == nil {
		return nil, "", "", fmt.Errorf("%w: file is required", ErrInvalidRequest)
	}
	if declared > max {
		return nil, "", "", fmt.Errorf("%w: file is too large", ErrInvalidRequest)
	}
	body, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, "", "", err
	}
	if len(body) == 0 || int64(len(body)) > max {
		return nil, "", "", fmt.Errorf("%w: file is too large", ErrInvalidRequest)
	}
	mime := http.DetectContentType(body)
	if !allowed[mime] {
		return nil, "", "", fmt.Errorf("%w: unsupported file type", ErrInvalidRequest)
	}
	return body, mime, sha256Hex(body), nil
}

func sha256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func pgText(s string) pgtype.Text {
	if strings.TrimSpace(s) == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: strings.TrimSpace(s), Valid: true}
}

func trimUA(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 512 {
		return s[:512]
	}
	return s
}

func ptrSignature(s model.Signature) *model.Signature { return &s }

func signerView(row db.ContractSigner) model.Signer {
	v := model.Signer{UUID: row.Uuid, Role: row.Role, Name: row.Name}
	if row.UserID.Valid {
		id := row.UserID.Int64
		v.UserID = &id
	}
	if row.PhoneE164.Valid {
		v.PhoneE164 = row.PhoneE164.String
	}
	if row.OtpVerifiedAt.Valid {
		t := row.OtpVerifiedAt.Time
		v.OTPVerifiedAt = &t
	}
	if row.SignedAt.Valid {
		t := row.SignedAt.Time
		v.SignedAt = &t
	}
	return v
}

func signatureView(row db.ContractSignature) model.Signature {
	v := model.Signature{
		UUID: row.Uuid, StorageKey: row.StorageKey, SHA256: row.Sha256,
		CreatedAt: row.CreatedAt.Time,
	}
	if row.IpAddress != nil {
		v.IPAddress = row.IpAddress.String()
	}
	if row.UserAgent.Valid {
		v.UserAgent = row.UserAgent.String
	}
	return v
}

func mediaView(row db.ContractMedium) model.Media {
	v := model.Media{
		UUID: row.Uuid, StorageKey: row.StorageKey, MIMEType: row.MimeType,
		SizeBytes: row.SizeBytes, SHA256: row.Sha256, SortOrder: row.SortOrder,
		CreatedAt: row.CreatedAt.Time,
	}
	if row.Title.Valid {
		v.Title = row.Title.String
	}
	return v
}

// PrepareHTML sanitizes template HTML and rejects unknown variables.
func PrepareHTML(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("%w: html is required", ErrInvalidRequest)
	}
	if len(raw) > MaxTemplateBytes {
		return "", fmt.Errorf("%w: html exceeds %d bytes", ErrInvalidRequest, MaxTemplateBytes)
	}
	html := pdfrender.SanitizeHTML(raw)
	if unknown := UnknownVariables(html); len(unknown) > 0 {
		return "", &UnknownVariablesError{Keys: unknown}
	}
	return html, nil
}

// UnknownVariables returns disallowed placeholders.
func UnknownVariables(html string) []string {
	allowed := map[string]bool{}
	for _, v := range variables {
		allowed[v.Key] = true
	}
	var out []string
	for _, p := range msgtemplate.Placeholders(html) {
		if !allowed[p] {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// NormalizeLocale maps accepted locale spellings to DB values.
func NormalizeLocale(raw string) string {
	s := strings.TrimSpace(strings.ReplaceAll(raw, "_", "-"))
	if s == "" {
		return ""
	}
	if strings.EqualFold(s, "zh") || strings.EqualFold(s, "zh-cn") {
		return "zh-CN"
	}
	if i := strings.Index(s, "-"); i > 0 {
		s = s[:i]
	}
	s = strings.ToLower(s)
	for _, l := range []string{"tr", "en", "bg", "de", "el", "uk", "ru", "fr", "es", "it", "az", "ar"} {
		if s == l {
			return l
		}
	}
	return ""
}

func optBool(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *v, Valid: true}
}

func normalizeKind(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	switch s {
	case model.KindVehicleIntake, model.KindServiceSale:
		return s, nil
	default:
		return "", fmt.Errorf("%w: unknown kind", ErrInvalidRequest)
	}
}

func normalizeName(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" || len(s) > 150 {
		return "", fmt.Errorf("%w: name is required (max 150)", ErrInvalidRequest)
	}
	return s, nil
}

func lexicalBytes(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("%w: lexical_json is not valid JSON", ErrInvalidRequest)
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("%w: lexical_json is not valid JSON", ErrInvalidRequest)
	}
	if _, ok := v.(map[string]any); !ok {
		return nil, fmt.Errorf("%w: lexical_json must be an object", ErrInvalidRequest)
	}
	if len(raw) > 4*MaxTemplateBytes {
		return nil, fmt.Errorf("%w: lexical_json is too large", ErrInvalidRequest)
	}
	return raw, nil
}

func int8(id int64) pgtype.Int8 {
	return pgtype.Int8{Int64: id, Valid: id > 0}
}

func (s *Service) view(ctx context.Context, row db.ContractTemplate, withLocales bool) (model.Template, error) {
	v := model.Template{
		ID: row.ID, UUID: row.Uuid, Name: row.Name, Kind: row.Kind, IsDefault: row.IsDefault,
		OTPRequired: row.OtpRequired, SignatureRequired: row.SignatureRequired,
		IsActive: row.IsActive, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	if withLocales {
		rows, err := s.repo.Queries().ListContractTemplateLocales(ctx, row.ID)
		if err != nil {
			return model.Template{}, err
		}
		v.Locales = make([]model.TemplateLocale, 0, len(rows))
		for _, r := range rows {
			v.Locales = append(v.Locales, localeView(r))
		}
	}
	return v, nil
}

func localeView(row db.ContractTemplateLocale) model.TemplateLocale {
	v := model.TemplateLocale{
		UUID: row.Uuid, Locale: row.Locale, HTML: row.Html, Version: row.Version,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	if len(row.LexicalJson) > 0 {
		v.LexicalJSON = json.RawMessage(row.LexicalJson)
	}
	return v
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func text(group, key, tr, en string) model.Variable {
	return model.Variable{Key: key, Group: group, LabelTR: tr, LabelEN: en}
}

var variables = []model.Variable{
	text("customer", "customer_name", "Müşteri adı", "Customer name"),
	text("customer", "customer_phone", "Müşteri telefonu", "Customer phone"),
	text("customer", "customer_email", "Müşteri e-postası", "Customer e-mail"),
	text("vehicle", "plate", "Plaka", "Plate"),
	text("vehicle", "vin", "VIN", "VIN"),
	text("vehicle", "vehicle_label", "Araç", "Vehicle"),
	text("service", "service_no", "Hizmet no", "Service no"),
	text("service", "service_name", "Hizmet adı", "Service name"),
	text("service", "package", "Paket", "Package"),
	text("service", "start_date", "Başlangıç tarihi", "Start date"),
	text("service", "end_date", "Bitiş tarihi", "End date"),
	text("service", "price", "Fiyat", "Price"),
	text("organization", "org_name", "Organizasyon adı", "Organization name"),
	text("organization", "org_phone", "Organizasyon telefonu", "Organization phone"),
	text("organization", "org_email", "Organizasyon e-postası", "Organization e-mail"),
	text("organization", "org_address", "Organizasyon adresi", "Organization address"),
	text("staff", "staff_name", "Personel adı", "Staff name"),
	text("date", "today", "Bugün", "Today"),
}

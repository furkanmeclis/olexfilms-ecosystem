// Package usecase implements contract template CRUD and rendering.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrNotFound       = errors.New("contract template not found")
	ErrInvalidRequest = errors.New("invalid contract template request")
	ErrInUse          = errors.New("contract template is in use")
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

// ListFilter filters templates.
type ListFilter struct {
	Kind       string
	ActiveOnly bool
}

// Service is the contract template use case.
type Service struct{ repo *repository.Store }

// New creates a service.
func New(repo *repository.Store) *Service { return &Service{repo: repo} }

// Variables returns the fixed allow-list.
func (s *Service) Variables() []model.Variable { return variables }

// List returns templates for the active brand.
func (s *Service) List(ctx context.Context, c Caller, f ListFilter) ([]model.Template, error) {
	var kind pgtype.Text
	if strings.TrimSpace(f.Kind) != "" {
		k, err := normalizeKind(f.Kind)
		if err != nil {
			return nil, err
		}
		kind = pgtype.Text{String: k, Valid: true}
	}
	rows, err := s.repo.Queries().ListContractTemplates(ctx, db.ListContractTemplatesParams{
		BrandID: c.BrandID, Kind: kind, ActiveOnly: f.ActiveOnly,
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
		UUID: row.Uuid, Name: row.Name, Kind: row.Kind, IsDefault: row.IsDefault,
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
	text("service", "package", "Paket", "Package"),
	text("organization", "org_name", "Organizasyon adı", "Organization name"),
	text("organization", "org_phone", "Organizasyon telefonu", "Organization phone"),
	text("organization", "org_email", "Organizasyon e-postası", "Organization e-mail"),
	text("organization", "org_address", "Organizasyon adresi", "Organization address"),
	text("staff", "staff_name", "Personel adı", "Staff name"),
	text("date", "today", "Bugün", "Today"),
}

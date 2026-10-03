package usecase

// Label templates (TEC-202): how unit and location labels are printed.
// Templates belong to the active organization (center or distributor, the
// warehouse side is brand-independent, K20). Reads need stock.read, writes
// stock.write; the dealer has no warehouse and no label printing (K12).

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/labels"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// Label errors; the handler maps them to HTTP codes.
var (
	// ErrLabelsForbidden: the organization type has no label printing
	// (dealer, K12) or the grant does not reach the active organization.
	ErrLabelsForbidden = errors.New("stock: labels are not available for this organization")
	// ErrTemplateNameTaken: another template of the organization has the name.
	ErrTemplateNameTaken = errors.New("stock: label template name already used")
)

// Template limits.
const (
	maxTemplateNameLen = 100
	maxLogoTextLen     = 100
	// MaxLogoImageLen bounds the logo data URI (about 150 KB of PNG).
	MaxLogoImageLen = 200_000
)

// LabelTemplates implements the template CRUD.
type LabelTemplates struct {
	q *db.Queries
}

// NewLabelTemplates builds the use case.
func NewLabelTemplates(q *db.Queries) *LabelTemplates { return &LabelTemplates{q: q} }

// labelOrg returns the active organization id when the caller may use
// labels there: center or distributor, inside the grant scope.
func labelOrg(c Caller) (int64, error) {
	switch c.Org.OrgType {
	case rbac.OrgTypeCenter, rbac.OrgTypeDistributor:
	default:
		return 0, ErrLabelsForbidden
	}
	if c.Org.InternalID == 0 || !c.Filter.AllowsOrg(c.Org.InternalID, c.Org.BrandID) || c.Filter.UserOnly() {
		return 0, ErrLabelsForbidden
	}
	return c.Org.InternalID, nil
}

// TemplateInput creates or replaces a template. Nil pointers take the
// defaults of the kind.
type TemplateInput struct {
	Name         string
	Kind         string
	Symbology    string
	LogoMode     string
	LogoText     string
	LogoImage    string
	WidthMm      *string
	HeightMm     *string
	Columns      *int
	ShowName     *bool
	ShowCodeText *bool
	IsDefault    bool
	Active       *bool
}

// List returns the organization's templates, optionally of one kind.
func (s *LabelTemplates) List(ctx context.Context, c Caller, kind string) ([]model.LabelTemplate, error) {
	org, err := labelOrg(c)
	if err != nil {
		return nil, err
	}
	kind = strings.TrimSpace(kind)
	if kind != "" && kind != labels.KindUnit && kind != labels.KindLocation {
		return nil, invalid("kind", "must be unit or location")
	}
	rows, err := s.q.ListLabelTemplates(ctx, db.ListLabelTemplatesParams{OrganizationID: org, Kind: pgText(kind)})
	if err != nil {
		return nil, fmt.Errorf("stock: templates: %w", err)
	}
	out := make([]model.LabelTemplate, 0, len(rows))
	for _, t := range rows {
		out = append(out, TemplateView(t))
	}
	return out, nil
}

// Get returns one template of the organization.
func (s *LabelTemplates) Get(ctx context.Context, c Caller, id uuid.UUID) (model.LabelTemplate, error) {
	org, err := labelOrg(c)
	if err != nil {
		return model.LabelTemplate{}, err
	}
	t, err := s.template(ctx, org, id)
	if err != nil {
		return model.LabelTemplate{}, err
	}
	return TemplateView(t), nil
}

func (s *LabelTemplates) template(ctx context.Context, org int64, id uuid.UUID) (db.LabelTemplate, error) {
	t, err := s.q.GetLabelTemplateByUUID(ctx, db.GetLabelTemplateByUUIDParams{Uuid: id, OrganizationID: org})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.LabelTemplate{}, ErrNotFound
	}
	if err != nil {
		return db.LabelTemplate{}, fmt.Errorf("stock: template: %w", err)
	}
	return t, nil
}

// Create saves a new template.
func (s *LabelTemplates) Create(ctx context.Context, c Caller, in TemplateInput) (model.LabelTemplate, error) {
	org, err := labelOrg(c)
	if err != nil {
		return model.LabelTemplate{}, err
	}
	kind := strings.TrimSpace(in.Kind)
	if kind == "" {
		kind = labels.KindUnit
	}
	if kind != labels.KindUnit && kind != labels.KindLocation {
		return model.LabelTemplate{}, invalid("kind", "must be unit or location")
	}
	base := labels.DefaultUnit
	if kind == labels.KindLocation {
		base = labels.DefaultLocation
	}
	arg, err := templateArgs(in, base)
	if err != nil {
		return model.LabelTemplate{}, err
	}
	t, err := s.q.CreateLabelTemplate(ctx, db.CreateLabelTemplateParams{
		OrganizationID: org, Name: arg.Name, Kind: kind, Symbology: arg.Symbology,
		LogoMode: arg.LogoMode, LogoText: arg.LogoText, LogoImage: arg.LogoImage,
		WidthMm: arg.WidthMm, HeightMm: arg.HeightMm, GridColumns: arg.GridColumns,
		ShowName: arg.ShowName, ShowCodeText: arg.ShowCodeText, IsDefault: false, Active: arg.Active,
	})
	if err != nil {
		return model.LabelTemplate{}, templateDBErr(err)
	}
	if in.IsDefault {
		if t, err = s.setDefault(ctx, org, t); err != nil {
			return model.LabelTemplate{}, err
		}
	}
	return TemplateView(t), nil
}

// Update replaces the editable fields of a template (kind is fixed).
func (s *LabelTemplates) Update(ctx context.Context, c Caller, id uuid.UUID, in TemplateInput) (model.LabelTemplate, error) {
	org, err := labelOrg(c)
	if err != nil {
		return model.LabelTemplate{}, err
	}
	cur, err := s.template(ctx, org, id)
	if err != nil {
		return model.LabelTemplate{}, err
	}
	arg, err := templateArgs(in, templateOf(cur))
	if err != nil {
		return model.LabelTemplate{}, err
	}
	arg.ID, arg.OrganizationID = cur.ID, org
	arg.IsDefault = cur.IsDefault && in.IsDefault
	if in.IsDefault && !cur.IsDefault {
		// Clear the previous default first so the partial unique index holds.
		if err := s.q.ClearDefaultLabelTemplate(ctx, db.ClearDefaultLabelTemplateParams{OrganizationID: org, Kind: cur.Kind, KeepID: cur.ID}); err != nil {
			return model.LabelTemplate{}, fmt.Errorf("stock: clear default: %w", err)
		}
		arg.IsDefault = true
	}
	t, err := s.q.UpdateLabelTemplate(ctx, arg)
	if err != nil {
		return model.LabelTemplate{}, templateDBErr(err)
	}
	return TemplateView(t), nil
}

// Delete removes a template; batches that referenced it keep no link.
func (s *LabelTemplates) Delete(ctx context.Context, c Caller, id uuid.UUID) error {
	org, err := labelOrg(c)
	if err != nil {
		return err
	}
	n, err := s.q.DeleteLabelTemplate(ctx, db.DeleteLabelTemplateParams{Uuid: id, OrganizationID: org})
	if err != nil {
		return fmt.Errorf("stock: delete template: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *LabelTemplates) setDefault(ctx context.Context, org int64, t db.LabelTemplate) (db.LabelTemplate, error) {
	if err := s.q.ClearDefaultLabelTemplate(ctx, db.ClearDefaultLabelTemplateParams{OrganizationID: org, Kind: t.Kind, KeepID: t.ID}); err != nil {
		return t, fmt.Errorf("stock: clear default: %w", err)
	}
	arg := db.UpdateLabelTemplateParams{
		ID: t.ID, OrganizationID: org, Name: t.Name, Symbology: t.Symbology, LogoMode: t.LogoMode,
		LogoText: t.LogoText, LogoImage: t.LogoImage, WidthMm: t.WidthMm, HeightMm: t.HeightMm,
		GridColumns: t.GridColumns, ShowName: t.ShowName, ShowCodeText: t.ShowCodeText,
		IsDefault: true, Active: t.Active,
	}
	t, err := s.q.UpdateLabelTemplate(ctx, arg)
	if err != nil {
		return t, templateDBErr(err)
	}
	return t, nil
}

// templateArgs validates the input against base (the kind's defaults or
// the current row) and returns the update parameters (id/org unset).
func templateArgs(in TemplateInput, base labels.Template) (db.UpdateLabelTemplateParams, error) {
	var arg db.UpdateLabelTemplateParams
	name := strings.TrimSpace(in.Name)
	switch {
	case name == "":
		return arg, invalid("name", "is required")
	case utf8.RuneCountInString(name) > maxTemplateNameLen:
		return arg, invalid("name", "is too long")
	}
	arg.Name = name
	arg.Symbology = strings.TrimSpace(in.Symbology)
	if arg.Symbology == "" {
		arg.Symbology = base.Symbology
	}
	if arg.Symbology != labels.SymbologyCode128 && arg.Symbology != labels.SymbologyQR {
		return arg, invalid("symbology", "must be code128 or qr")
	}
	if base.Kind == labels.KindLocation && arg.Symbology != labels.SymbologyQR {
		return arg, invalid("symbology", "location labels are QR codes")
	}
	arg.LogoMode = strings.TrimSpace(in.LogoMode)
	if arg.LogoMode == "" {
		arg.LogoMode = base.LogoMode
	}
	switch arg.LogoMode {
	case labels.LogoNone:
	case labels.LogoText:
		txt := strings.TrimSpace(in.LogoText)
		if txt == "" {
			txt = base.LogoText
		}
		if txt == "" || utf8.RuneCountInString(txt) > maxLogoTextLen {
			return arg, invalid("logo_text", "is required for logo_mode text (at most 100 characters)")
		}
		arg.LogoText = pgText(txt)
	case labels.LogoImage:
		img := strings.TrimSpace(in.LogoImage)
		if img == "" {
			img = base.LogoImage
		}
		if err := checkLogoImage(img); err != nil {
			return arg, err
		}
		arg.LogoImage = pgText(img)
	default:
		return arg, invalid("logo_mode", "must be none, text or image")
	}
	var err error
	if arg.WidthMm, err = mmValue("width_mm", in.WidthMm, base.WidthMm, 20, 200); err != nil {
		return arg, err
	}
	if arg.HeightMm, err = mmValue("height_mm", in.HeightMm, base.HeightMm, 10, 200); err != nil {
		return arg, err
	}
	cols := base.Columns
	if in.Columns != nil {
		cols = *in.Columns
	}
	if cols < 1 || cols > 8 {
		return arg, invalid("columns", "must be between 1 and 8")
	}
	arg.GridColumns = int16(cols)
	arg.ShowName = boolOr(in.ShowName, base.ShowName)
	arg.ShowCodeText = boolOr(in.ShowCodeText, base.ShowCodeText)
	arg.Active = boolOr(in.Active, true)
	return arg, nil
}

func boolOr(p *bool, def bool) bool {
	if p != nil {
		return *p
	}
	return def
}

// checkLogoImage accepts a PNG or JPEG data URI with valid base64.
func checkLogoImage(img string) error {
	if img == "" {
		return invalid("logo_image", "is required for logo_mode image")
	}
	if len(img) > MaxLogoImageLen {
		return invalid("logo_image", "is too large")
	}
	var payload string
	switch {
	case strings.HasPrefix(img, "data:image/png;base64,"):
		payload = strings.TrimPrefix(img, "data:image/png;base64,")
	case strings.HasPrefix(img, "data:image/jpeg;base64,"):
		payload = strings.TrimPrefix(img, "data:image/jpeg;base64,")
	default:
		return invalid("logo_image", "must be a data:image/png or data:image/jpeg base64 URI")
	}
	if _, err := base64.StdEncoding.DecodeString(payload); err != nil || payload == "" {
		return invalid("logo_image", "is not valid base64")
	}
	return nil
}

// mmValue parses a millimeter value with one decimal.
func mmValue(field string, raw *string, def, lo, hi float64) (pgtype.Numeric, error) {
	v := def
	if raw != nil {
		f, err := strconv.ParseFloat(strings.TrimSpace(*raw), 64)
		if err != nil {
			return pgtype.Numeric{}, invalid(field, "must be a number of millimeters")
		}
		v = f
	}
	if v < lo || v > hi {
		return pgtype.Numeric{}, invalid(field, fmt.Sprintf("must be between %.0f and %.0f mm", lo, hi))
	}
	tenths := int64(v*10 + 0.5)
	return pgtype.Numeric{Int: big.NewInt(tenths), Exp: -1, Valid: true}, nil
}

// numericFloat converts a NUMERIC to float64 (0 when null).
func numericFloat(n pgtype.Numeric) float64 {
	if !n.Valid || n.Int == nil {
		return 0
	}
	f, _ := new(big.Float).SetRat(numericRat(n)).Float64()
	return f
}

func numericRat(n pgtype.Numeric) *big.Rat {
	r := new(big.Rat).SetInt(n.Int)
	if n.Exp != 0 {
		p := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(abs(n.Exp))), nil)
		if n.Exp > 0 {
			r.Mul(r, new(big.Rat).SetInt(p))
		} else {
			r.Quo(r, new(big.Rat).SetInt(p))
		}
	}
	return r
}

// numericString1 renders a NUMERIC with one decimal.
func numericString1(n pgtype.Numeric) string {
	if !n.Valid || n.Int == nil {
		return "0.0"
	}
	return numericRat(n).FloatString(1)
}

func templateDBErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_label_templates_org_name" {
		return ErrTemplateNameTaken
	}
	return fmt.Errorf("stock: template: %w", err)
}

// templateOf converts a row into the render template.
func templateOf(t db.LabelTemplate) labels.Template {
	return labels.Template{
		Kind: t.Kind, Symbology: t.Symbology, LogoMode: t.LogoMode,
		LogoText: t.LogoText.String, LogoImage: t.LogoImage.String,
		WidthMm: numericFloat(t.WidthMm), HeightMm: numericFloat(t.HeightMm), Columns: int(t.GridColumns),
		ShowName: t.ShowName, ShowCodeText: t.ShowCodeText,
	}
}

// TemplateView renders a template for the API.
func TemplateView(t db.LabelTemplate) model.LabelTemplate {
	return model.LabelTemplate{
		UUID: t.Uuid, Name: t.Name, Kind: t.Kind, Symbology: t.Symbology, LogoMode: t.LogoMode,
		LogoText: textPtr(t.LogoText), LogoImage: textPtr(t.LogoImage),
		WidthMm: numericString1(t.WidthMm), HeightMm: numericString1(t.HeightMm), Columns: int(t.GridColumns),
		ShowName: t.ShowName, ShowCodeText: t.ShowCodeText, IsDefault: t.IsDefault, Active: t.Active,
		CreatedAt: tsTime(t.CreatedAt), UpdatedAt: tsTime(t.UpdatedAt),
	}
}

package geo

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrInvalidPlate: the plate does not match its country's format.
var ErrInvalidPlate = errors.New("geo: plate does not match the country format")

// PlateFormat is the public plate format projection. The plate country is
// picked on the vehicle, independent of the customer's country/locale.
type PlateFormat struct {
	CountryISO2     string `json:"country_iso2"`
	CountryNameEn   string `json:"country_name_en"`
	CountryNameTr   string `json:"country_name_tr"`
	Regex           string `json:"regex"`
	InputMask       string `json:"input_mask"`
	Example         string `json:"example"`
	CountryLabel    string `json:"country_label"`
	StripColor      string `json:"strip_color"`
	BackgroundColor string `json:"background_color"`
	TextColor       string `json:"text_color"`
	IsActive        bool   `json:"is_active"`
	SortOrder       int32  `json:"sort_order"`
}

// PlateCheck is the result of a plate validation.
type PlateCheck struct {
	Country    string `json:"country"`
	Normalized string `json:"normalized"`
	Valid      bool   `json:"valid"`
}

// NormalizePlate upper-cases a plate and drops separators (spaces, dashes,
// dots, the Chinese middle dot), the compact form the regex runs on.
func NormalizePlate(raw string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(raw)) {
		switch {
		case unicode.IsSpace(r), r == '-', r == '.', r == '·', r == '_':
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// CompilePlateRegex compiles a format regex (RE2, Go regexp) and requires it
// to be anchored so a partial match never validates a plate.
func CompilePlateRegex(expr string) (*regexp.Regexp, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" || !strings.HasPrefix(expr, "^") || !strings.HasSuffix(expr, "$") {
		return nil, fmt.Errorf("%w: regex must be anchored with ^ and $", ErrInvalid)
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, fmt.Errorf("%w: regex is not RE2 compatible: %v", ErrInvalid, err)
	}
	return re, nil
}

// MatchPlate validates a plate against a regex on its compact form.
func MatchPlate(expr, plate string) (string, bool, error) {
	re, err := CompilePlateRegex(expr)
	if err != nil {
		return "", false, err
	}
	n := NormalizePlate(plate)
	return n, n != "" && re.MatchString(n), nil
}

func mapPlate(f db.PlateFormat, iso2, nameEn, nameTr string) PlateFormat {
	return PlateFormat{
		CountryISO2: iso2, CountryNameEn: nameEn, CountryNameTr: nameTr,
		Regex: f.Regex, InputMask: f.InputMask, Example: f.Example, CountryLabel: f.CountryLabel,
		StripColor: f.StripColor, BackgroundColor: f.BackgroundColor, TextColor: f.TextColor,
		IsActive: f.IsActive, SortOrder: f.SortOrder,
	}
}

// PlateFormats lists formats (active only unless all is set).
func (s *Service) PlateFormats(ctx context.Context, all bool) ([]PlateFormat, error) {
	rows, err := s.q.ListPlateFormats(ctx, !all)
	if err != nil {
		return nil, err
	}
	out := make([]PlateFormat, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapPlate(db.PlateFormat{
			Regex: r.Regex, InputMask: r.InputMask, Example: r.Example, CountryLabel: r.CountryLabel,
			StripColor: r.StripColor, BackgroundColor: r.BackgroundColor, TextColor: r.TextColor,
			IsActive: r.IsActive, SortOrder: r.SortOrder,
		}, r.CountryIso2, r.CountryNameEn, r.CountryNameTr))
	}
	return out, nil
}

// ValidatePlate checks a plate against the active format of its plate
// country. A country without a format, or an inactive one, is ErrNotFound;
// a mismatch returns the check with ErrInvalidPlate.
func (s *Service) ValidatePlate(ctx context.Context, iso2, plate string) (PlateCheck, error) {
	code, err := NormalizeISO2(iso2)
	if err != nil {
		return PlateCheck{}, err
	}
	f, err := s.q.GetPlateFormatByCountry(ctx, code)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PlateCheck{}, fmt.Errorf("%w: no plate format for %s", ErrNotFound, code)
		}
		return PlateCheck{}, err
	}
	if !f.IsActive {
		return PlateCheck{}, fmt.Errorf("%w: plate format for %s is inactive", ErrNotFound, code)
	}
	n, ok, err := MatchPlate(f.Regex, plate)
	if err != nil {
		return PlateCheck{}, err
	}
	check := PlateCheck{Country: code, Normalized: n, Valid: ok}
	if !ok {
		return check, ErrInvalidPlate
	}
	return check, nil
}

// PlateFormatInput creates or patches a format; nil fields keep their value.
type PlateFormatInput struct {
	Regex           *string
	InputMask       *string
	Example         *string
	CountryLabel    *string
	StripColor      *string
	BackgroundColor *string
	TextColor       *string
	IsActive        *bool
	SortOrder       *int32
}

var colorRe = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

func (in PlateFormatInput) validate(create bool) error {
	if create && (in.Regex == nil || in.CountryLabel == nil) {
		return fmt.Errorf("%w: regex and country_label are required", ErrInvalid)
	}
	if in.Regex != nil {
		re, err := CompilePlateRegex(*in.Regex)
		if err != nil {
			return err
		}
		if in.Example != nil && strings.TrimSpace(*in.Example) != "" && !re.MatchString(NormalizePlate(*in.Example)) {
			return fmt.Errorf("%w: example does not match the regex", ErrInvalid)
		}
	}
	if in.CountryLabel != nil {
		l := strings.TrimSpace(*in.CountryLabel)
		if l == "" || len(l) > 4 {
			return fmt.Errorf("%w: country_label must be 1-4 characters", ErrInvalid)
		}
	}
	for _, c := range []*string{in.StripColor, in.BackgroundColor, in.TextColor} {
		if c != nil && !colorRe.MatchString(*c) {
			return fmt.Errorf("%w: colors must be #RRGGBB", ErrInvalid)
		}
	}
	if in.InputMask != nil && len(*in.InputMask) > 64 {
		return fmt.Errorf("%w: input_mask max 64 characters", ErrInvalid)
	}
	if in.Example != nil && len(*in.Example) > 32 {
		return fmt.Errorf("%w: example max 32 characters", ErrInvalid)
	}
	return nil
}

func strOr(p *string, def string) string {
	if p == nil {
		return def
	}
	return strings.TrimSpace(*p)
}

func narg(p *string) pgtype.Text {
	if p == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: strings.TrimSpace(*p), Valid: true}
}

// CreatePlateFormat adds the format of a country.
func (s *Service) CreatePlateFormat(ctx context.Context, iso2 string, in PlateFormatInput) (PlateFormat, error) {
	c, err := s.CountryByISO2(ctx, iso2)
	if err != nil {
		return PlateFormat{}, err
	}
	if err := in.validate(true); err != nil {
		return PlateFormat{}, err
	}
	active := true
	if in.IsActive != nil {
		active = *in.IsActive
	}
	var sort int32
	if in.SortOrder != nil {
		sort = *in.SortOrder
	}
	f, err := s.q.CreatePlateFormat(ctx, db.CreatePlateFormatParams{
		CountryID: c.ID, Regex: strOr(in.Regex, ""), InputMask: strOr(in.InputMask, ""),
		Example: strOr(in.Example, ""), CountryLabel: strOr(in.CountryLabel, ""),
		StripColor: strOr(in.StripColor, "#003399"), BackgroundColor: strOr(in.BackgroundColor, "#FFFFFF"),
		TextColor: strOr(in.TextColor, "#000000"), IsActive: active, SortOrder: sort,
	})
	if err != nil {
		return PlateFormat{}, mapWriteErr(err)
	}
	return mapPlate(f, c.Iso2, c.NameEn, c.NameTr), nil
}

// UpdatePlateFormat patches the format of a country.
func (s *Service) UpdatePlateFormat(ctx context.Context, iso2 string, in PlateFormatInput) (PlateFormat, error) {
	c, err := s.CountryByISO2(ctx, iso2)
	if err != nil {
		return PlateFormat{}, err
	}
	if err := in.validate(false); err != nil {
		return PlateFormat{}, err
	}
	if in.Example != nil && in.Regex == nil {
		// Validate the new example against the stored regex.
		cur, err := s.q.GetPlateFormatByCountry(ctx, c.Iso2)
		if err == nil {
			if _, ok, _ := MatchPlate(cur.Regex, *in.Example); !ok && strings.TrimSpace(*in.Example) != "" {
				return PlateFormat{}, fmt.Errorf("%w: example does not match the regex", ErrInvalid)
			}
		}
	}
	params := db.UpdatePlateFormatParams{
		CountryID: c.ID, Regex: narg(in.Regex), InputMask: narg(in.InputMask), Example: narg(in.Example),
		CountryLabel: narg(in.CountryLabel), StripColor: narg(in.StripColor),
		BackgroundColor: narg(in.BackgroundColor), TextColor: narg(in.TextColor),
	}
	if in.IsActive != nil {
		params.IsActive = pgtype.Bool{Bool: *in.IsActive, Valid: true}
	}
	if in.SortOrder != nil {
		params.SortOrder = pgtype.Int4{Int32: *in.SortOrder, Valid: true}
	}
	f, err := s.q.UpdatePlateFormat(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PlateFormat{}, fmt.Errorf("%w: plate format", ErrNotFound)
		}
		return PlateFormat{}, mapWriteErr(err)
	}
	return mapPlate(f, c.Iso2, c.NameEn, c.NameTr), nil
}

// DeletePlateFormat removes the format of a country.
func (s *Service) DeletePlateFormat(ctx context.Context, iso2 string) error {
	c, err := s.CountryByISO2(ctx, iso2)
	if err != nil {
		return err
	}
	n, err := s.q.DeletePlateFormat(ctx, c.ID)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: plate format", ErrNotFound)
	}
	return nil
}

// UnknownPlateFormatsError: a reorder named countries without a format.
type UnknownPlateFormatsError struct{ Countries []string }

func (e *UnknownPlateFormatsError) Error() string {
	return "geo: no plate format for " + strings.Join(e.Countries, ", ")
}

// PlateFormatOrderStep is the sort_order gap ReorderPlateFormats writes.
const PlateFormatOrderStep = 10

// ReorderPlateFormats applies a drag-and-drop order (TEC-367, PUT
// /v1/platform/plate-formats/order). countries are ISO2 codes in their new
// order; they may be a subset (a filtered table): the named formats keep
// the positions they occupy among all formats and are rearranged inside
// them. Every format is then renumbered 10, 20, ... in one transaction.
// countries must be valid, distinct ISO2 codes (the handler checks).
func (s *Service) ReorderPlateFormats(ctx context.Context, countries []string) ([]PlateFormat, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)
	rows, err := qtx.ListPlateFormats(ctx, false)
	if err != nil {
		return nil, err
	}
	pos := make(map[string]int, len(rows))
	for i, r := range rows {
		pos[r.CountryIso2] = i
	}
	var unknown []string
	slots := make([]int, 0, len(countries))
	for _, c := range countries {
		i, ok := pos[c]
		if !ok {
			unknown = append(unknown, c)
			continue
		}
		slots = append(slots, i)
	}
	if len(unknown) > 0 {
		return nil, &UnknownPlateFormatsError{Countries: unknown}
	}
	slices.Sort(slots)
	order := slices.Clone(rows)
	for k, c := range countries {
		order[slots[k]] = rows[pos[c]]
	}
	for i, r := range order {
		if err := qtx.SetPlateFormatSortOrder(ctx, db.SetPlateFormatSortOrderParams{
			CountryID: r.CountryID, SortOrder: int32((i + 1) * PlateFormatOrderStep),
		}); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.PlateFormats(ctx, true)
}

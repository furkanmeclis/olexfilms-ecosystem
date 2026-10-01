package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Organization types in the tree (center -> distributor -> dealer).
const (
	TypeCenter      = "center"
	TypeDistributor = "distributor"
	TypeDealer      = "dealer"
)

var (
	// ErrBrandMismatch: the organization belongs to another brand than the
	// request domain (K1, K20).
	ErrBrandMismatch = errors.New("organization brand does not match request brand")
	// ErrBrandUnresolved: the request has no brand (catalog unavailable).
	ErrBrandUnresolved = errors.New("request brand is not resolved")
	// ErrBrandInactive: the request brand does not accept this action yet (K2).
	ErrBrandInactive = errors.New("brand is not active")
)

// validStatuses are the statuses platform admins may set. pending/expired are
// legacy values kept for backward compatibility.
var validStatuses = map[string]struct{}{
	"pending": {}, "active": {}, "read_only": {}, "suspended": {}, "expired": {},
}

// ParentRef is the parent (supplier) of an organization.
type ParentRef struct {
	UUID uuid.UUID `json:"uuid"`
	Slug string    `json:"slug,omitempty"`
	Name string    `json:"name"`
}

// BrandRef is a brand projection embedded in organization payloads.
type BrandRef struct {
	Slug string `json:"slug"`
	Name string `json:"name,omitempty"`
}

// PublicBrand is the brand resolved for the current domain.
type PublicBrand struct {
	UUID   uuid.UUID `json:"uuid"`
	Slug   string    `json:"slug"`
	Name   string    `json:"name"`
	Status string    `json:"status"`
}

// RequestBrand returns the brand of the request context.
func RequestBrand(ctx context.Context) (brandctx.Brand, error) {
	b, ok := brandctx.From(ctx)
	if !ok {
		return brandctx.Brand{}, ErrBrandUnresolved
	}
	return b, nil
}

// PublicBrandFromContext returns the request brand for GET /v1/public/brand.
func PublicBrandFromContext(ctx context.Context) (PublicBrand, error) {
	b, err := RequestBrand(ctx)
	if err != nil {
		return PublicBrand{}, err
	}
	return PublicBrand{UUID: b.UUID, Slug: b.Slug, Name: b.Name, Status: b.Status}, nil
}

func validChildType(parentType, childType string) bool {
	switch childType {
	case TypeDistributor:
		return parentType == TypeCenter
	case TypeDealer:
		return parentType == TypeCenter || parentType == TypeDistributor
	default:
		return false
	}
}

// SupplierOf returns the organization one level up in the tree (K9).
// A center has no supplier and returns nil.
func (s *Service) SupplierOf(ctx context.Context, orgID int64) (*db.Organization, error) {
	row, err := s.q.SupplierOf(ctx, orgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// Descendants returns every organization below orgID.
func (s *Service) Descendants(ctx context.Context, orgID int64) ([]db.Organization, error) {
	return s.q.Descendants(ctx, orgID)
}

// brandOrg loads an organization of the request brand; others read as not found.
func (s *Service) brandOrg(ctx context.Context, id uuid.UUID) (db.GetOrganizationTreeByUUIDRow, error) {
	brand, err := RequestBrand(ctx)
	if err != nil {
		return db.GetOrganizationTreeByUUIDRow{}, err
	}
	row, err := s.q.GetOrganizationTreeByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return row, ErrNotFound
		}
		return row, err
	}
	if row.Organization.BrandID != brand.ID {
		return row, ErrNotFound
	}
	return row, nil
}

// Children lists direct children of an organization (platform).
func (s *Service) Children(ctx context.Context, id uuid.UUID) ([]Organization, error) {
	parent, err := s.brandOrg(ctx, id)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListOrganizationChildren(ctx, pgtype.Int8{Int64: parent.Organization.ID, Valid: true})
	if err != nil {
		return nil, err
	}
	out := make([]Organization, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapTree(row.Organization, row.BrandSlug, row.ParentUuid, row.ParentName))
	}
	return out, nil
}

// ChangeParent moves an organization under another parent. Platform-only (K25);
// the open-balance transfer record arrives with the accounting module.
func (s *Service) ChangeParent(ctx context.Context, id, parentUUID uuid.UUID) error {
	child, err := s.brandOrg(ctx, id)
	if err != nil {
		return err
	}
	if child.Organization.Type == TypeCenter {
		return fmt.Errorf("%w: a center has no parent", ErrInvalidRequest)
	}
	parent, err := s.brandOrg(ctx, parentUUID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return fmt.Errorf("%w: parent organization not found", ErrInvalidRequest)
		}
		return err
	}
	if !validChildType(parent.Organization.Type, child.Organization.Type) {
		return fmt.Errorf("%w: a %s cannot be placed under a %s", ErrInvalidRequest,
			child.Organization.Type, parent.Organization.Type)
	}
	if parent.Organization.ID == child.Organization.ID {
		return fmt.Errorf("%w: an organization cannot be its own parent", ErrInvalidRequest)
	}
	below, err := s.q.Descendants(ctx, child.Organization.ID)
	if err != nil {
		return err
	}
	for _, d := range below {
		if d.ID == parent.Organization.ID {
			return fmt.Errorf("%w: parent cannot be a descendant", ErrInvalidRequest)
		}
	}
	_, err = s.q.UpdateOrganizationParent(ctx, db.UpdateOrganizationParentParams{
		ID: child.Organization.ID, ParentID: pgtype.Int8{Int64: parent.Organization.ID, Valid: true},
	})
	return err
}

func mapTree(row db.Organization, brandSlug string, parentUUID pgtype.UUID, parentName pgtype.Text) Organization {
	o := mapOrganization(row)
	o.Brand = BrandRef{Slug: brandSlug}
	if parentUUID.Valid {
		o.Parent = &ParentRef{UUID: uuid.UUID(parentUUID.Bytes), Name: parentName.String}
	}
	return o
}

func decodeSettings(raw []byte) map[string]any {
	out := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}

// placementResult is where a new organization lands in the tree.
type placementResult struct {
	Type      string
	Parent    db.Organization
	BrandSlug string
	Currency  string
	Locale    string
	Timezone  string
	Settings  []byte
}

// placement validates type/parent for a platform-created organization. The
// brand is always the request brand; the parent defaults to the brand center.
func (s *Service) placement(ctx context.Context, in RegisterInput) (placementResult, error) {
	brand, err := RequestBrand(ctx)
	if err != nil {
		return placementResult{}, err
	}
	typ := strings.TrimSpace(in.Type)
	if typ == "" {
		typ = TypeDealer
	}
	if typ == TypeCenter {
		return placementResult{}, fmt.Errorf("%w: a brand has exactly one center", ErrInvalidRequest)
	}
	if typ != TypeDistributor && typ != TypeDealer {
		return placementResult{}, fmt.Errorf("%w: invalid organization type", ErrInvalidRequest)
	}
	var parent db.Organization
	if in.ParentUUID != nil {
		row, err := s.brandOrg(ctx, *in.ParentUUID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return placementResult{}, fmt.Errorf("%w: parent organization not found", ErrInvalidRequest)
			}
			return placementResult{}, err
		}
		parent = row.Organization
	} else {
		parent, err = s.q.GetBrandCenter(ctx, brand.ID)
		if err != nil {
			return placementResult{}, fmt.Errorf("brand center: %w", err)
		}
	}
	if !validChildType(parent.Type, typ) {
		return placementResult{}, fmt.Errorf("%w: a %s cannot be placed under a %s", ErrInvalidRequest, typ, parent.Type)
	}
	if in.RegisterAsWarehouse && typ != TypeDistributor {
		return placementResult{}, fmt.Errorf("%w: register_as_warehouse is only for distributors", ErrInvalidRequest)
	}
	out := placementResult{
		Type: typ, Parent: parent, BrandSlug: brand.Slug,
		Currency: parent.Currency, Locale: parent.Locale, Timezone: parent.Timezone,
		Settings: []byte("{}"),
	}
	if c := strings.ToUpper(strings.TrimSpace(in.Currency)); c != "" {
		if !validCurrency(c) {
			return placementResult{}, fmt.Errorf("%w: currency must be an ISO 4217 code", ErrInvalidRequest)
		}
		out.Currency = c
	}
	if l := strings.TrimSpace(in.Locale); l != "" {
		out.Locale = l
	}
	if tz := strings.TrimSpace(in.Timezone); tz != "" {
		if _, err := time.LoadLocation(tz); err != nil {
			return placementResult{}, fmt.Errorf("%w: invalid timezone", ErrInvalidRequest)
		}
		out.Timezone = tz
	}
	if in.RegisterAsWarehouse {
		// K4: the warehouse itself is created by the warehouse module (F1).
		out.Settings = []byte(`{"register_as_warehouse":true}`)
	}
	return out, nil
}

func withBrandParent(o Organization, brandSlug string, parent db.Organization) Organization {
	o.Brand = BrandRef{Slug: brandSlug}
	o.Parent = &ParentRef{UUID: parent.Uuid, Slug: parent.Slug, Name: parent.Name}
	return o
}

// checkBrand fails closed when the request brand is missing or different.
func checkBrand(ctx context.Context, orgBrandID int64) error {
	brand, ok := brandctx.From(ctx)
	if !ok {
		return ErrBrandUnresolved
	}
	if brand.ID != orgBrandID {
		return ErrBrandMismatch
	}
	return nil
}

func validCurrency(c string) bool {
	if len(c) != 3 {
		return false
	}
	for _, r := range c {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

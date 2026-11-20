package usecase

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-1265, kullanıcı kararı 6 (TEC-1253): a Glorian dealer matches an
// organization by the Glorian dealer code; the phone is only the fallback
// and an admin can link a dealer by hand. The rule lives in the
// glorian_dealer_matches view (migration 000206), which the order outbound
// reads too, so this screen shows exactly where an order would go.

// Match types of a dealer.
const (
	MatchManual    = "manual"
	MatchCode      = "code"
	MatchPhone     = "phone"
	MatchAmbiguous = "ambiguous"
	MatchNone      = "none"
)

// MatchTypes lists the match types (the match_type filter).
var MatchTypes = []string{MatchManual, MatchCode, MatchPhone, MatchAmbiguous, MatchNone}

// ErrDealerCodeTaken: another organization of the brand has the code (409).
var ErrDealerCodeTaken = errors.New("glorian admin: dealer code already used")

// ErrOrganizationLinked: the organization is already linked by hand to
// another dealer of the connection (409).
var ErrOrganizationLinked = errors.New("glorian admin: organization already linked")

const (
	maxDealerCode         = 64
	dealerOrgPickerLimit  = 50
	maxDealerOrgPickerLen = 100
)

// DealersSortSpec is the sort whitelist of GET .../dealers.
var DealersSortSpec = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"name": "name", "dealer_code": "dealer_code", "match_type": "match_type", "synced_at": "synced_at",
	},
	Default: apiquery.SortField{Field: "name"},
}

// DealerOrganization is the organization a dealer matches, or a picker
// option.
type DealerOrganization struct {
	UUID              uuid.UUID `json:"uuid"`
	Name              string    `json:"name"`
	City              string    `json:"city"`
	Phone             string    `json:"phone"`
	Type              string    `json:"type,omitempty"`
	GlorianDealerCode *string   `json:"glorian_dealer_code"`
}

// DealerView is one Glorian dealer with its match.
type DealerView struct {
	UUID         uuid.UUID           `json:"uuid"`
	RemoteID     string              `json:"remote_id"`
	Name         string              `json:"name"`
	DealerCode   *string             `json:"dealer_code"`
	Phone        *string             `json:"phone"`
	Active       bool                `json:"active"`
	MatchType    string              `json:"match_type"`
	Organization *DealerOrganization `json:"organization"`
	SyncedAt     *time.Time          `json:"synced_at"`
	LinkedAt     *time.Time          `json:"linked_at"`
}

// DealerFilter is the list query.
type DealerFilter struct {
	Q          string
	MatchTypes []string
	Active     *bool
	SortKey    string
	SortDesc   bool
	Limit      int
	Offset     int
}

// DealerLinkInput links a dealer by hand; a nil organization clears it.
type DealerLinkInput struct {
	OrganizationUUID *uuid.UUID `json:"organization_uuid"`
}

// DealerCodeInput sets an organization's Glorian dealer code; nil or an
// empty string clears it.
type DealerCodeInput struct {
	GlorianDealerCode *string `json:"glorian_dealer_code"`
}

// ListDealers lists the Glorian dealers of the brand's connection with
// their match.
func (s *Service) ListDealers(ctx context.Context, brand db.Brand, f DealerFilter) ([]DealerView, int64, error) {
	conn, err := s.connection(ctx, brand.ID)
	if err != nil {
		return nil, 0, err
	}
	types := f.MatchTypes
	if types == nil {
		types = []string{}
	}
	var active pgtype.Bool
	if f.Active != nil {
		active = pgtype.Bool{Bool: *f.Active, Valid: true}
	}
	q := optText(strings.TrimSpace(f.Q))
	rows, err := s.q.ListGlorianDealerMatches(ctx, db.ListGlorianDealerMatchesParams{
		ConnectionID: conn.ID, Q: q, MatchTypes: types, Active: active,
		SortKey: f.SortKey, SortDesc: f.SortDesc,
		PageLimit: int32(ClampLimit(f.Limit)), PageOffset: int32(max(f.Offset, 0)),
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountGlorianDealerMatches(ctx, db.CountGlorianDealerMatchesParams{
		ConnectionID: conn.ID, Q: q, MatchTypes: types, Active: active,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]DealerView, 0, len(rows))
	for _, r := range rows {
		out = append(out, dealerView(r))
	}
	return out, total, nil
}

func dealerView(r db.ListGlorianDealerMatchesRow) DealerView {
	v := DealerView{
		UUID: r.Uuid, RemoteID: r.RemoteID, Name: r.Name, DealerCode: text(r.DealerCode),
		Phone: text(r.PhoneE164), Active: r.Active, MatchType: r.MatchType,
		SyncedAt: ts(r.SyncedAt), LinkedAt: ts(r.LinkedAt),
	}
	if r.OrgUuid.Valid {
		v.Organization = &DealerOrganization{
			UUID: r.OrgUuid.Bytes, Name: r.OrgName.String, City: r.OrgCity.String,
			Phone: r.OrgPhone.String, GlorianDealerCode: text(r.OrgDealerCode),
		}
	}
	return v
}

// dealer returns the view of one dealer of conn.
func (s *Service) dealer(ctx context.Context, conn db.IntegrationConnection, partyID int64) (DealerView, error) {
	rows, err := s.q.ListGlorianDealerMatches(ctx, db.ListGlorianDealerMatchesParams{
		ConnectionID: conn.ID, MatchTypes: []string{}, PartyID: pgtype.Int8{Int64: partyID, Valid: true},
		SortKey: "name", PageLimit: 1,
	})
	if err != nil {
		return DealerView{}, err
	}
	if len(rows) == 0 {
		return DealerView{}, ErrNotFound
	}
	return dealerView(rows[0]), nil
}

// LinkDealer links a dealer to an organization of the brand by hand (nil
// clears the manual link; the code and phone match apply again).
func (s *Service) LinkDealer(ctx context.Context, brand db.Brand, id uuid.UUID, in DealerLinkInput) (DealerView, error) {
	conn, err := s.connection(ctx, brand.ID)
	if err != nil {
		return DealerView{}, err
	}
	if _, err := s.q.GetIntegrationExternalPartyByUUID(ctx, db.GetIntegrationExternalPartyByUUIDParams{ConnectionID: conn.ID, Uuid: id}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DealerView{}, ErrNotFound
		}
		return DealerView{}, err
	}
	var org pgtype.Int8
	if in.OrganizationUUID != nil {
		o, err := s.q.GetBrandOrganizationByUUID(ctx, db.GetBrandOrganizationByUUIDParams{Uuid: *in.OrganizationUUID, BrandID: conn.BrandID})
		if errors.Is(err, pgx.ErrNoRows) {
			return DealerView{}, invalid("organization_uuid", "is not an organization of the brand")
		}
		if err != nil {
			return DealerView{}, err
		}
		org = pgtype.Int8{Int64: o.ID, Valid: true}
	}
	p, err := s.q.SetIntegrationExternalPartyLink(ctx, db.SetIntegrationExternalPartyLinkParams{
		ConnectionID: conn.ID, Uuid: id, LinkedOrganizationID: org,
	})
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return DealerView{}, ErrOrganizationLinked
	}
	if err != nil {
		return DealerView{}, err
	}
	return s.dealer(ctx, conn, p.ID)
}

// DealerOrganizations searches the brand's organizations (the picker of
// the manual link, at most 50).
func (s *Service) DealerOrganizations(ctx context.Context, brand db.Brand, q string) ([]DealerOrganization, error) {
	conn, err := s.connection(ctx, brand.ID)
	if err != nil {
		return nil, err
	}
	q = strings.TrimSpace(q)
	if utf8.RuneCountInString(q) > maxDealerOrgPickerLen {
		return nil, invalid("q", "must be at most 100 characters")
	}
	rows, err := s.q.SearchGlorianDealerOrganizations(ctx, db.SearchGlorianDealerOrganizationsParams{
		BrandID: conn.BrandID, Q: optText(q), RowLimit: dealerOrgPickerLimit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]DealerOrganization, 0, len(rows))
	for _, o := range rows {
		out = append(out, DealerOrganization{
			UUID: o.Uuid, Name: o.Name, City: o.City, Phone: o.Phone, Type: o.Type,
			GlorianDealerCode: text(o.GlorianDealerCode),
		})
	}
	return out, nil
}

// SetDealerCode sets the Glorian dealer code of an organization of the
// brand (unique in the brand, case-insensitive; empty clears it).
func (s *Service) SetDealerCode(ctx context.Context, brand db.Brand, orgUUID uuid.UUID, in DealerCodeInput) (DealerOrganization, error) {
	conn, err := s.connection(ctx, brand.ID)
	if err != nil {
		return DealerOrganization{}, err
	}
	o, err := s.q.GetBrandOrganizationByUUID(ctx, db.GetBrandOrganizationByUUIDParams{Uuid: orgUUID, BrandID: conn.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return DealerOrganization{}, ErrNotFound
	}
	if err != nil {
		return DealerOrganization{}, err
	}
	var code pgtype.Text
	if in.GlorianDealerCode != nil {
		c := strings.TrimSpace(*in.GlorianDealerCode)
		if utf8.RuneCountInString(c) > maxDealerCode {
			return DealerOrganization{}, invalid("glorian_dealer_code", "must be at most 64 characters")
		}
		code = optText(c)
	}
	o, err = s.q.SetOrganizationGlorianDealerCode(ctx, db.SetOrganizationGlorianDealerCodeParams{ID: o.ID, GlorianDealerCode: code})
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return DealerOrganization{}, ErrDealerCodeTaken
	}
	if err != nil {
		return DealerOrganization{}, err
	}
	return DealerOrganization{
		UUID: o.Uuid, Name: o.Name, City: o.City, Phone: o.Phone, Type: o.Type,
		GlorianDealerCode: text(o.GlorianDealerCode),
	}, nil
}

package usecase

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Audience types. Panel user audiences are business notifications and need
// no marketing consent (F4 QUESTIONS S5); dealer_users is open to the center
// and distributors, distributor_users to the center only.
const (
	AudienceCustomers        = "customers"
	AudienceDealerUsers      = "dealer_users"
	AudienceDistributorUsers = "distributor_users"
)

// Warranty states of the audience filter.
const (
	WarrantyActive   = "active"
	WarrantyExpiring = "expiring" // active, ends within 30 days
	WarrantyExpired  = "expired"  // has an ended warranty and no active one
)

var (
	audienceTypes    = []string{AudienceCustomers, AudienceDealerUsers, AudienceDistributorUsers}
	warrantyStatuses = []string{WarrantyActive, WarrantyExpiring, WarrantyExpired}
)

const (
	maxFilterValues = 200
	sampleSize      = 10
	dateLayout      = "2006-01-02"
)

// AudienceFilter is the validated audience definition stored in
// campaigns.audience_filter. Dates are inclusive calendar days (UTC).
type AudienceFilter struct {
	AudienceType      string      `json:"audience_type"`
	CountryIDs        []int64     `json:"country_ids"`
	ProvinceIDs       []int64     `json:"province_ids"`
	DistrictIDs       []int64     `json:"district_ids"`
	OrganizationUUIDs []uuid.UUID `json:"organization_uuids"`
	LastServiceFrom   *string     `json:"last_service_from"`
	LastServiceTo     *string     `json:"last_service_to"`
	CarBrandUUIDs     []uuid.UUID `json:"car_brand_uuids"`
	ProductUUIDs      []uuid.UUID `json:"product_uuids"`
	CategoryUUIDs     []uuid.UUID `json:"category_uuids"`
	WarrantyStatuses  []string    `json:"warranty_statuses"`
	Locales           []string    `json:"locales"`
}

// normalizedLists replaces nil lists with empty ones (stable JSON).
func (f AudienceFilter) normalizedLists() AudienceFilter {
	if f.CountryIDs == nil {
		f.CountryIDs = []int64{}
	}
	if f.ProvinceIDs == nil {
		f.ProvinceIDs = []int64{}
	}
	if f.DistrictIDs == nil {
		f.DistrictIDs = []int64{}
	}
	if f.OrganizationUUIDs == nil {
		f.OrganizationUUIDs = []uuid.UUID{}
	}
	if f.CarBrandUUIDs == nil {
		f.CarBrandUUIDs = []uuid.UUID{}
	}
	if f.ProductUUIDs == nil {
		f.ProductUUIDs = []uuid.UUID{}
	}
	if f.CategoryUUIDs == nil {
		f.CategoryUUIDs = []uuid.UUID{}
	}
	if f.WarrantyStatuses == nil {
		f.WarrantyStatuses = []string{}
	}
	if f.Locales == nil {
		f.Locales = []string{}
	}
	return f
}

// AllowedAudienceTypes are the audience types an organization type may
// target: a dealer reaches only its customers.
func AllowedAudienceTypes(orgType string) []string {
	switch orgType {
	case rbac.OrgTypeCenter:
		return audienceTypes
	case rbac.OrgTypeDistributor:
		return []string{AudienceCustomers, AudienceDealerUsers}
	default:
		return []string{AudienceCustomers}
	}
}

// validateFilter checks the filter for a campaign of org and returns it
// normalized (deduplicated, sorted lists; empty lists kept).
func (s *Service) validateFilter(ctx context.Context, q *db.Queries, org db.Organization, f AudienceFilter) (AudienceFilter, error) {
	f.AudienceType = strings.TrimSpace(f.AudienceType)
	if f.AudienceType == "" {
		return f, invalid("audience_filter.audience_type", "is required")
	}
	if !contains(audienceTypes, f.AudienceType) {
		return f, invalid("audience_filter.audience_type", "must be customers, dealer_users or distributor_users")
	}
	if !contains(AllowedAudienceTypes(org.Type), f.AudienceType) {
		return f, invalid("audience_filter.audience_type", "is not available for this organization")
	}
	var err error
	if f.CountryIDs, err = positiveIDs("audience_filter.country_ids", f.CountryIDs); err != nil {
		return f, err
	}
	if f.ProvinceIDs, err = positiveIDs("audience_filter.province_ids", f.ProvinceIDs); err != nil {
		return f, err
	}
	if f.DistrictIDs, err = positiveIDs("audience_filter.district_ids", f.DistrictIDs); err != nil {
		return f, err
	}
	if f.OrganizationUUIDs, err = uniqueUUIDs("audience_filter.organization_uuids", f.OrganizationUUIDs); err != nil {
		return f, err
	}
	if f.CarBrandUUIDs, err = uniqueUUIDs("audience_filter.car_brand_uuids", f.CarBrandUUIDs); err != nil {
		return f, err
	}
	if f.ProductUUIDs, err = uniqueUUIDs("audience_filter.product_uuids", f.ProductUUIDs); err != nil {
		return f, err
	}
	if f.CategoryUUIDs, err = uniqueUUIDs("audience_filter.category_uuids", f.CategoryUUIDs); err != nil {
		return f, err
	}
	if f.WarrantyStatuses, err = enumList("audience_filter.warranty_statuses", f.WarrantyStatuses, warrantyStatuses); err != nil {
		return f, err
	}
	locales := make([]string, 0, len(i18n.Supported))
	for _, l := range i18n.Supported {
		locales = append(locales, string(l))
	}
	if f.Locales, err = enumList("audience_filter.locales", f.Locales, locales); err != nil {
		return f, err
	}
	from, err := parseDay("audience_filter.last_service_from", f.LastServiceFrom)
	if err != nil {
		return f, err
	}
	to, err := parseDay("audience_filter.last_service_to", f.LastServiceTo)
	if err != nil {
		return f, err
	}
	if from != nil && to != nil && from.After(*to) {
		return f, invalid("audience_filter.last_service_to", "must not be before last_service_from")
	}
	f.LastServiceFrom, f.LastServiceTo = dayString(from), dayString(to)
	if f.AudienceType != AudienceCustomers {
		switch {
		case f.LastServiceFrom != nil || f.LastServiceTo != nil:
			return f, invalid("audience_filter.last_service_from", "applies to customer audiences only")
		case len(f.CarBrandUUIDs) > 0:
			return f, invalid("audience_filter.car_brand_uuids", "applies to customer audiences only")
		case len(f.ProductUUIDs) > 0:
			return f, invalid("audience_filter.product_uuids", "applies to customer audiences only")
		case len(f.CategoryUUIDs) > 0:
			return f, invalid("audience_filter.category_uuids", "applies to customer audiences only")
		case len(f.WarrantyStatuses) > 0:
			return f, invalid("audience_filter.warranty_statuses", "applies to customer audiences only")
		}
	}
	if len(f.OrganizationUUIDs) > 0 {
		if _, err := s.filterOrgIDs(ctx, q, org, f); err != nil {
			return f, err
		}
	}
	return f.normalizedLists(), nil
}

// reach returns the organizations a campaign of org reaches (K20): nil for
// the center (whole brand), the subtree for a distributor, the dealer
// itself otherwise.
func (s *Service) reach(ctx context.Context, q *db.Queries, org db.Organization) ([]int64, error) {
	switch org.Type {
	case rbac.OrgTypeCenter:
		return nil, nil
	case rbac.OrgTypeDistributor:
		below, err := q.Descendants(ctx, org.ID)
		if err != nil {
			return nil, err
		}
		ids := []int64{org.ID}
		for _, o := range below {
			ids = append(ids, o.ID)
		}
		return ids, nil
	default:
		return []int64{org.ID}, nil
	}
}

// filterOrgIDs resolves organization_uuids; every organization must be in
// the campaign reach.
func (s *Service) filterOrgIDs(ctx context.Context, q *db.Queries, org db.Organization, f AudienceFilter) ([]int64, error) {
	if len(f.OrganizationUUIDs) == 0 {
		return nil, nil
	}
	reach, err := s.reach(ctx, q, org)
	if err != nil {
		return nil, err
	}
	rows, err := q.ResolveCampaignOrganizations(ctx, db.ResolveCampaignOrganizationsParams{
		BrandID: org.BrandID, Uuids: f.OrganizationUUIDs,
	})
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		if reach != nil && !containsID(reach, r.ID) {
			continue
		}
		ids = append(ids, r.ID)
	}
	if len(ids) != len(f.OrganizationUUIDs) {
		return nil, invalid("audience_filter.organization_uuids", "contains an organization outside the campaign reach")
	}
	return ids, nil
}

// Recipient is one resolved audience member.
type Recipient struct {
	UserID     int64
	UserUUID   uuid.UUID
	Name       string
	Surname    string
	Email      string
	PhoneE164  string
	Locale     string
	PushTokens int32
	// Channels the user is reachable on among the campaign channels.
	Channels []string
	// Channel preferences (customer_profiles.notification_prefs) and the
	// marketing opt-out of the phone number (the snapshot records why a
	// channel is skipped).
	PrefPush, PrefWhatsApp, PrefEmail bool
	MarketingOptedOut                 bool
}

// Audience is the resolved audience of a campaign.
type Audience struct {
	// Eligible members in user id order (consent rule applied).
	Recipients []Recipient
	// ExcludedNoConsent: customers without an accepted marketing consent.
	ExcludedNoConsent int
	// ExcludedOptedOut: consenting customers opted out of marketing.
	ExcludedOptedOut int
}

// ResolveAudience evaluates the stored filter of a campaign (F4-04d takes
// the recipient snapshot from the same result).
func (s *Service) ResolveAudience(ctx context.Context, q *db.Queries, row db.Campaign) (Audience, error) {
	var f AudienceFilter
	if err := json.Unmarshal(row.AudienceFilter, &f); err != nil {
		return Audience{}, err
	}
	org, err := q.GetOrganizationByID(ctx, row.OrganizationID)
	if err != nil {
		return Audience{}, err
	}
	if !contains(AllowedAudienceTypes(org.Type), f.AudienceType) {
		return Audience{}, invalid("audience_filter.audience_type", "is not available for this organization")
	}
	reach, err := s.reach(ctx, q, org)
	if err != nil {
		return Audience{}, err
	}
	orgIDs, err := s.filterOrgIDs(ctx, q, org, f)
	if err != nil {
		return Audience{}, err
	}
	from, _ := parseDay("last_service_from", f.LastServiceFrom)
	to, _ := parseDay("last_service_to", f.LastServiceTo)
	var before *time.Time
	if to != nil {
		b := to.AddDate(0, 0, 1)
		before = &b
	}
	rows, err := q.ListCampaignAudience(ctx, db.ListCampaignAudienceParams{
		BrandID: row.BrandID, AudienceType: f.AudienceType, ReachOrgIds: reach, OrganizationIds: orgIDs,
		CountryKeys: idKeys(f.CountryIDs), ProvinceKeys: idKeys(f.ProvinceIDs), DistrictKeys: idKeys(f.DistrictIDs),
		LastServiceFrom: pgTime(from), LastServiceBefore: pgTime(before),
		CarBrandUuids: f.CarBrandUUIDs, ProductUuids: f.ProductUUIDs, CategoryUuids: f.CategoryUUIDs,
		WarrantyStatuses: f.WarrantyStatuses, Now: pgtype.Timestamptz{Time: s.now(), Valid: true},
	})
	if err != nil {
		return Audience{}, err
	}
	var out Audience
	for _, r := range rows {
		if len(f.Locales) > 0 && !contains(f.Locales, r.Locale) {
			continue
		}
		if f.AudienceType == AudienceCustomers {
			if !r.MarketingAccepted {
				out.ExcludedNoConsent++
				continue
			}
			if r.MarketingOptedOut {
				out.ExcludedOptedOut++
				continue
			}
		}
		rc := Recipient{
			UserID: r.UserID, UserUUID: r.UserUuid, Name: r.Name, Surname: r.Surname, Email: r.Email,
			PhoneE164: r.PhoneE164, Locale: r.Locale, PushTokens: r.PushTokens, Channels: []string{},
			PrefPush: r.PrefPush, PrefWhatsApp: r.PrefWhatsapp, PrefEmail: r.PrefEmail,
			MarketingOptedOut: r.MarketingOptedOut,
		}
		for _, ch := range row.Channels {
			ok := false
			switch ch {
			case ChannelPush:
				ok = r.PushTokens > 0 && r.PrefPush
			case ChannelWhatsApp:
				ok = r.PhoneE164 != "" && r.PrefWhatsapp
			case ChannelEmail:
				ok = r.Email != "" && r.PrefEmail
			}
			if ok {
				rc.Channels = append(rc.Channels, ch)
			}
		}
		out.Recipients = append(out.Recipients, rc)
	}
	return out, nil
}

// Preview is the answer of POST /v1/campaigns/{uuid}/preview.
type Preview struct {
	// Total eligible members (consent rule applied).
	Total int `json:"total"`
	// Locales is the distribution by resolved user locale (K10).
	Locales []LocaleCount `json:"locales"`
	// Channels: members reachable per selected channel (push token,
	// WhatsApp E.164 number, e-mail address).
	Channels []ChannelCount `json:"channels"`
	// Unreachable: eligible members with no reachable selected channel.
	Unreachable int          `json:"unreachable"`
	Excluded    ExcludedInfo `json:"excluded"`
	// MissingLocales: audience locales whose content is missing or
	// incomplete for a selected channel (submission answers 422).
	MissingLocales []string       `json:"missing_locales"`
	Sample         []SampleMember `json:"sample"`
}

// LocaleCount is one locale of the distribution.
type LocaleCount struct {
	Locale string `json:"locale"`
	Count  int    `json:"count"`
}

// ChannelCount is the reach of one channel.
type ChannelCount struct {
	Channel   string `json:"channel"`
	Reachable int    `json:"reachable"`
}

// ExcludedInfo counts customers left out by the consent rule.
type ExcludedInfo struct {
	Total     int `json:"total"`
	NoConsent int `json:"no_consent"`
	OptedOut  int `json:"opted_out"`
}

// SampleMember is a masked audience member.
type SampleMember struct {
	Name     string   `json:"name"`
	Phone    *string  `json:"phone"`
	Email    *string  `json:"email"`
	Locale   string   `json:"locale"`
	Channels []string `json:"channels"`
}

// Preview resolves the audience of a campaign in the caller's read scope.
func (s *Service) Preview(ctx context.Context, c Caller, id uuid.UUID) (Preview, error) {
	row, err := s.load(ctx, s.q, c, id)
	if err != nil {
		return Preview{}, err
	}
	aud, err := s.ResolveAudience(ctx, s.q, row)
	if err != nil {
		return Preview{}, err
	}
	contents, err := s.q.ListCampaignContents(ctx, row.ID)
	if err != nil {
		return Preview{}, err
	}
	out := Preview{
		Total: len(aud.Recipients), Locales: []LocaleCount{}, Channels: []ChannelCount{},
		Excluded: ExcludedInfo{
			Total: aud.ExcludedNoConsent + aud.ExcludedOptedOut, NoConsent: aud.ExcludedNoConsent, OptedOut: aud.ExcludedOptedOut,
		},
		MissingLocales: missingLocales(row.Channels, aud, contents),
		Sample:         []SampleMember{},
	}
	byLocale := map[string]int{}
	byChannel := map[string]int{}
	for _, r := range aud.Recipients {
		byLocale[r.Locale]++
		if len(r.Channels) == 0 {
			out.Unreachable++
		}
		for _, ch := range r.Channels {
			byChannel[ch]++
		}
		if len(out.Sample) < sampleSize {
			out.Sample = append(out.Sample, sampleMember(r))
		}
	}
	for l, n := range byLocale {
		out.Locales = append(out.Locales, LocaleCount{Locale: l, Count: n})
	}
	sort.Slice(out.Locales, func(i, j int) bool {
		if out.Locales[i].Count != out.Locales[j].Count {
			return out.Locales[i].Count > out.Locales[j].Count
		}
		return out.Locales[i].Locale < out.Locales[j].Locale
	})
	for _, ch := range row.Channels {
		out.Channels = append(out.Channels, ChannelCount{Channel: ch, Reachable: byChannel[ch]})
	}
	return out, nil
}

// RequireLocalized is the mandatory localization gate: every locale of the
// audience needs a complete content for all selected channels. Submission
// for approval and sending (F4-04c/d) call it; a gap answers
// *LocaleMissingError (422 CAMPAIGN_LOCALE_MISSING).
func (s *Service) RequireLocalized(ctx context.Context, q *db.Queries, row db.Campaign) error {
	aud, err := s.ResolveAudience(ctx, q, row)
	if err != nil {
		return err
	}
	contents, err := q.ListCampaignContents(ctx, row.ID)
	if err != nil {
		return err
	}
	if missing := missingLocales(row.Channels, aud, contents); len(missing) > 0 {
		return &LocaleMissingError{Locales: missing}
	}
	return nil
}

// CheckLocalized runs RequireLocalized for a campaign in the caller's scope.
func (s *Service) CheckLocalized(ctx context.Context, c Caller, id uuid.UUID) error {
	row, err := s.load(ctx, s.q, c, id)
	if err != nil {
		return err
	}
	return s.RequireLocalized(ctx, s.q, row)
}

func missingLocales(channels []string, aud Audience, contents []db.CampaignContent) []string {
	byLocale := map[string]db.CampaignContent{}
	for _, ct := range contents {
		byLocale[ct.Locale] = ct
	}
	seen := map[string]bool{}
	missing := []string{}
	for _, r := range aud.Recipients {
		if seen[r.Locale] {
			continue
		}
		seen[r.Locale] = true
		ct, ok := byLocale[r.Locale]
		if !ok || !complete(channels, ct.Title, ct.Body) {
			missing = append(missing, r.Locale)
		}
	}
	sort.Strings(missing)
	return missing
}

func sampleMember(r Recipient) SampleMember {
	m := SampleMember{Name: maskName(r.Name, r.Surname), Locale: r.Locale, Channels: r.Channels}
	if r.PhoneE164 != "" {
		p := maskPhone(r.PhoneE164)
		m.Phone = &p
	}
	if r.Email != "" {
		e := maskEmail(r.Email)
		m.Email = &e
	}
	return m
}

// maskName keeps the first letter of each name part: "Ayşe Yılmaz" →
// "A*** Y***".
func maskName(name, surname string) string {
	parts := strings.Fields(name + " " + surname)
	for i, p := range parts {
		r, _ := utf8.DecodeRuneInString(p)
		parts[i] = string(r) + "***"
	}
	return strings.Join(parts, " ")
}

// maskPhone keeps the country prefix and the last two digits:
// "+905551234567" → "+90*******67".
func maskPhone(e164 string) string {
	if len(e164) <= 5 {
		return strings.Repeat("*", len(e164))
	}
	return e164[:3] + strings.Repeat("*", len(e164)-5) + e164[len(e164)-2:]
}

// maskEmail keeps the first letter of the local part and the top-level
// domain: "ayse@example.com" → "a***@e***.com".
func maskEmail(email string) string {
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		return "***"
	}
	local, domain := email[:at], email[at+1:]
	r, _ := utf8.DecodeRuneInString(local)
	out := string(r) + "***@"
	dot := strings.LastIndex(domain, ".")
	if dot <= 0 {
		return out + "***"
	}
	d, _ := utf8.DecodeRuneInString(domain)
	return out + string(d) + "***" + domain[dot:]
}

func positiveIDs(field string, in []int64) ([]int64, error) {
	if len(in) > maxFilterValues {
		return nil, invalid(field, "has too many values")
	}
	seen := map[int64]bool{}
	out := make([]int64, 0, len(in))
	for _, id := range in {
		if id <= 0 {
			return nil, invalid(field, "must contain positive ids")
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func uniqueUUIDs(field string, in []uuid.UUID) ([]uuid.UUID, error) {
	if len(in) > maxFilterValues {
		return nil, invalid(field, "has too many values")
	}
	seen := map[uuid.UUID]bool{}
	out := make([]uuid.UUID, 0, len(in))
	for _, id := range in {
		if id == uuid.Nil {
			return nil, invalid(field, "must contain uuids")
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out, nil
}

func enumList(field string, in, allowed []string) ([]string, error) {
	seen := map[string]bool{}
	for _, v := range in {
		if !contains(allowed, v) {
			return nil, invalid(field, "must be one of "+strings.Join(allowed, ", "))
		}
		seen[v] = true
	}
	out := make([]string, 0, len(seen))
	for _, v := range allowed {
		if seen[v] {
			out = append(out, v)
		}
	}
	return out, nil
}

func parseDay(field string, v *string) (*time.Time, error) {
	if v == nil || strings.TrimSpace(*v) == "" {
		return nil, nil
	}
	t, err := time.Parse(dateLayout, strings.TrimSpace(*v))
	if err != nil {
		return nil, invalid(field, "must be a date (YYYY-MM-DD)")
	}
	return &t, nil
}

func dayString(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(dateLayout)
	return &s
}

func idKeys(ids []int64) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, strconv.FormatInt(id, 10))
	}
	return out
}

func containsID(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func pgTime(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

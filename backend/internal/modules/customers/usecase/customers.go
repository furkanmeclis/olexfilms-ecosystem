package usecase

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/crypto"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/password"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/phone"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// OrganizationLink is an organization serving the customer (inside the
// caller's scope only).
type OrganizationLink struct {
	UUID           uuid.UUID  `json:"uuid"`
	Name           string     `json:"name"`
	Type           string     `json:"type"`
	LinkedAt       time.Time  `json:"linked_at"`
	FirstServiceAt *time.Time `json:"first_service_at"`
}

// CustomerSummary is a list row. An anonymized customer is masked: the name
// is the localized "Anonymous" label and contact fields are null.
type CustomerSummary struct {
	UUID           uuid.UUID  `json:"uuid"`
	Name           string     `json:"name"`
	Surname        string     `json:"surname"`
	Email          *string    `json:"email"`
	Phone          *string    `json:"phone"`
	Status         string     `json:"status"`
	Anonymized     bool       `json:"anonymized"`
	Type           string     `json:"type"`
	CompanyName    *string    `json:"company_name"`
	Locale         *string    `json:"locale"`
	CreatedAt      time.Time  `json:"created_at"`
	LinkedAt       *time.Time `json:"linked_at"`
	FirstServiceAt *time.Time `json:"first_service_at"`
}

// CustomerDetail is one customer. Identity numbers are never returned in
// full (TEC-100 decision 3); only their last four characters.
type CustomerDetail struct {
	CustomerSummary
	TaxOffice         *string            `json:"tax_office"`
	NationalIDLast4   *string            `json:"national_id_last4"`
	TaxNoLast4        *string            `json:"tax_no_last4"`
	Address           json.RawMessage    `json:"address"`
	NotificationPrefs json.RawMessage    `json:"notification_prefs"`
	Editable          bool               `json:"editable"`
	IdentityEditable  bool               `json:"identity_editable"`
	Organizations     []OrganizationLink `json:"organizations"`
}

// CustomerWrite is the result of a create or update. ExistingUser: the phone
// already belonged to a user, who was linked. IgnoredFields lists submitted
// values that were kept as stored because the customer is shared with an
// organization outside the caller's scope (fill-only).
type CustomerWrite struct {
	CustomerDetail
	ExistingUser  bool     `json:"existing_user"`
	IgnoredFields []string `json:"ignored_fields"`
}

// CreateCustomerInput is POST /v1/customers.
type CreateCustomerInput struct {
	Phone             string          `json:"phone"`
	Name              string          `json:"name"`
	Surname           string          `json:"surname"`
	Email             string          `json:"email"`
	Locale            string          `json:"locale"`
	Type              string          `json:"type"`
	CompanyName       string          `json:"company_name"`
	TaxOffice         string          `json:"tax_office"`
	NationalID        string          `json:"national_id"`
	TaxNo             string          `json:"tax_no"`
	Address           json.RawMessage `json:"address"`
	NotificationPrefs map[string]bool `json:"notification_prefs"`
}

// UpdateCustomerInput is PATCH /v1/customers/{uuid}: absent keys keep the
// stored value, null (or "") clears it.
type UpdateCustomerInput struct {
	Name              Optional[string]          `json:"name"`
	Surname           Optional[string]          `json:"surname"`
	Email             Optional[string]          `json:"email"`
	Type              Optional[string]          `json:"type"`
	CompanyName       Optional[string]          `json:"company_name"`
	TaxOffice         Optional[string]          `json:"tax_office"`
	NationalID        Optional[string]          `json:"national_id"`
	TaxNo             Optional[string]          `json:"tax_no"`
	Address           Optional[json.RawMessage] `json:"address"`
	NotificationPrefs Optional[map[string]bool] `json:"notification_prefs"`
}

// ListFilter filters GET /v1/customers.
type ListFilter struct {
	Q      string
	Status string
	Limit  int32
	Offset int32
}

// profilePatch is a normalized profile change ("" clears a text field).
type profilePatch struct {
	Type        Optional[string]
	CompanyName Optional[string]
	TaxOffice   Optional[string]
	NationalID  Optional[string]
	TaxNo       Optional[string]
	Address     Optional[[]byte]
	Prefs       Optional[map[string]bool]
}

func (p profilePatch) needsPII() bool {
	return (p.NationalID.Set && p.NationalID.Value != nil && *p.NationalID.Value != "") ||
		(p.TaxNo.Set && p.TaxNo.Value != nil && *p.TaxNo.Value != "")
}

// normalizeProfilePatch validates the profile part of a create/update.
func normalizeProfilePatch(typ, company, taxOffice, nationalID, taxNo Optional[string],
	address Optional[json.RawMessage], prefs Optional[map[string]bool],
) (profilePatch, error) {
	var p profilePatch
	norm := func(o Optional[string], fn func(string) (string, error)) (Optional[string], error) {
		if !o.Set {
			return o, nil
		}
		raw := ""
		if o.Value != nil {
			raw = *o.Value
		}
		v, err := fn(raw)
		if err != nil {
			return o, err
		}
		return Of(v), nil
	}
	var err error
	if p.Type, err = norm(typ, normalizeCustomerType); err != nil {
		return p, err
	}
	if p.CompanyName, err = norm(company, func(s string) (string, error) {
		return normalizeText(s, "company_name", maxCompanyNameLen, "must be at most 200 characters")
	}); err != nil {
		return p, err
	}
	if p.TaxOffice, err = norm(taxOffice, func(s string) (string, error) {
		return normalizeText(s, "tax_office", maxTaxOfficeLen, "must be at most 150 characters")
	}); err != nil {
		return p, err
	}
	if p.NationalID, err = norm(nationalID, func(s string) (string, error) { return normalizeIdentityNumber(s, "national_id") }); err != nil {
		return p, err
	}
	if p.TaxNo, err = norm(taxNo, func(s string) (string, error) { return normalizeIdentityNumber(s, "tax_no") }); err != nil {
		return p, err
	}
	if address.Set {
		var raw json.RawMessage
		if address.Value != nil {
			raw = *address.Value
		}
		b, err := normalizeAddress(raw)
		if err != nil {
			return p, err
		}
		p.Address = Of(b)
	}
	if prefs.Set {
		m := map[string]bool{}
		if prefs.Value != nil {
			m = *prefs.Value
		}
		if _, err := mergeNotificationPrefs(nil, m); err != nil {
			return p, err
		}
		p.Prefs = Of(m)
	}
	return p, nil
}

// presentIfNonEmpty turns a create field into an Optional (empty = absent).
func presentIfNonEmpty(v string) Optional[string] {
	if strings.TrimSpace(v) == "" {
		return Optional[string]{}
	}
	return Of(v)
}

// --- Create ------------------------------------------------------------------

type createData struct {
	e164    string
	name    string
	surname string
	email   string
	locale  string
	profile profilePatch
}

// CreateCustomer creates or links the customer with the given phone in the
// active organization (one phone = one user, K11/K26). The phone is
// normalized to E.164 with the organization's country as default region
// (K29). An existing user keeps its name, e-mail and profile; only empty
// fields are filled (IgnoredFields lists the rest).
func (s *Service) CreateCustomer(ctx context.Context, c Caller, in CreateCustomerInput) (CustomerWrite, error) {
	if c.Org.InternalID == 0 || !c.Filter.AllowsOrg(c.Org.InternalID, c.Org.BrandID) {
		return CustomerWrite{}, ErrForbidden
	}
	iso2, err := s.q.GetOrganizationCountryISO2(ctx, c.Org.InternalID)
	if err != nil {
		return CustomerWrite{}, fmt.Errorf("customers: organization country: %w", err)
	}
	if strings.TrimSpace(in.Phone) == "" {
		return CustomerWrite{}, invalid("phone", "is required")
	}
	e164, err := phone.NormalizeE164(in.Phone, phone.Region(iso2))
	if err != nil || e164 == "" {
		return CustomerWrite{}, invalid("phone", "must be a valid phone number")
	}
	d := createData{e164: e164}
	if d.name, err = normalizeName(in.Name, "name"); err != nil {
		return CustomerWrite{}, err
	}
	if d.name == "" {
		return CustomerWrite{}, invalid("name", "is required")
	}
	if d.surname, err = normalizeName(in.Surname, "surname"); err != nil {
		return CustomerWrite{}, err
	}
	if d.email, err = normalizeEmail(in.Email); err != nil {
		return CustomerWrite{}, err
	}
	if strings.TrimSpace(in.Locale) != "" {
		l, ok := i18n.Parse(in.Locale)
		if !ok {
			return CustomerWrite{}, invalid("locale", "is not a supported language")
		}
		d.locale = string(l)
	}
	var addr Optional[json.RawMessage]
	if len(bytes.TrimSpace(in.Address)) > 0 {
		addr = Of(in.Address)
	}
	var prefs Optional[map[string]bool]
	if in.NotificationPrefs != nil {
		prefs = Of(in.NotificationPrefs)
	}
	if d.profile, err = normalizeProfilePatch(presentIfNonEmpty(in.Type), presentIfNonEmpty(in.CompanyName),
		presentIfNonEmpty(in.TaxOffice), presentIfNonEmpty(in.NationalID), presentIfNonEmpty(in.TaxNo), addr, prefs); err != nil {
		return CustomerWrite{}, err
	}
	if d.profile.needsPII() && s.pii == nil {
		return CustomerWrite{}, ErrPIIUnavailable
	}

	var (
		user     db.User
		existing bool
		ignored  []string
	)
	for attempt := 0; ; attempt++ {
		err = s.inTxRaw(ctx, func(q *db.Queries, tx pgx.Tx) error {
			var err error
			user, existing, ignored, err = s.createOnce(ctx, q, c, d)
			if err != nil || existing {
				return err
			}
			// TEC-164: a new customer gets the WhatsApp welcome with the
			// portal link (outbox -> notifications bus).
			return s.enqueueCreated(ctx, tx, c, user, d.profile)
		})
		// Two organizations creating the same new phone at once: the loser
		// hits the unique phone index and retries as "existing user".
		if attempt == 0 && isUniqueViolation(err, "uq_users_phone_e164_active") {
			continue
		}
		break
	}
	if err != nil {
		return CustomerWrite{}, err
	}
	s.indexCustomer(ctx, user.Uuid) // TEC-164: new link or new customer
	detail, err := s.detail(ctx, s.q, c, user)
	if err != nil {
		return CustomerWrite{}, err
	}
	if ignored == nil {
		ignored = []string{}
	}
	return CustomerWrite{CustomerDetail: detail, ExistingUser: existing, IgnoredFields: ignored}, nil
}

func (s *Service) createOnce(ctx context.Context, q *db.Queries, c Caller, d createData) (db.User, bool, []string, error) {
	var ignored []string
	user, err := q.GetUserByPhone(ctx, text(d.e164))
	existing := err == nil
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if d.email != "" {
			if _, err := q.GetUserByEmail(ctx, text(d.email)); err == nil {
				return db.User{}, false, nil, ErrEmailTaken
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return db.User{}, false, nil, fmt.Errorf("customers: email lookup: %w", err)
			}
		}
		hash, err := unusablePasswordHash()
		if err != nil {
			return db.User{}, false, nil, err
		}
		// The phone is not verified: the customer verifies it with the first
		// WhatsApp OTP (K26 claim).
		user, err = q.CreateUser(ctx, db.CreateUserParams{
			Email: text(d.email), PasswordHash: hash, Name: d.name, Surname: d.surname,
			Status: StatusActive, PhoneE164: text(d.e164),
		})
		if err != nil {
			if isUniqueViolation(err, "uq_users_email_active") {
				return db.User{}, false, nil, ErrEmailTaken
			}
			return db.User{}, false, nil, err
		}
		if d.locale != "" {
			if err := q.UpdateUserLocale(ctx, db.UpdateUserLocaleParams{ID: user.ID, Locale: text(d.locale)}); err != nil {
				return db.User{}, false, nil, err
			}
			user.Locale = text(d.locale)
		}
	case err != nil:
		return db.User{}, false, nil, fmt.Errorf("customers: phone lookup: %w", err)
	default:
		if err := writableUser(user); err != nil {
			return db.User{}, false, nil, err
		}
		var fill db.FillCustomerIdentityParams
		fill.ID = user.ID
		fill.Locale = text(d.locale)
		ignored, fill.Name, fill.Surname = fillNames(user, d.name, d.surname, ignored)
		if d.email != "" {
			switch {
			case user.Email.Valid:
				if user.Email.String != d.email {
					ignored = append(ignored, "email")
				}
			default:
				other, err := q.GetUserByEmail(ctx, text(d.email))
				switch {
				case err == nil && other.ID != user.ID:
					ignored = append(ignored, "email")
				case err != nil && !errors.Is(err, pgx.ErrNoRows):
					return db.User{}, false, nil, fmt.Errorf("customers: email lookup: %w", err)
				default:
					fill.Email = text(d.email)
				}
			}
		}
		user, err = q.FillCustomerIdentity(ctx, fill)
		if err != nil {
			return db.User{}, false, nil, err
		}
	}
	// Every customer holds the customer role (portal realm); an existing
	// staff account keeps its roles.
	if err := q.AssignUserRoleBySlug(ctx, db.AssignUserRoleBySlugParams{UserID: user.ID, Slug: rbac.RoleCustomer}); err != nil {
		return db.User{}, false, nil, err
	}
	profIgnored, err := s.applyProfile(ctx, q, user.ID, d.profile, false)
	if err != nil {
		return db.User{}, false, nil, err
	}
	ignored = append(ignored, profIgnored...)
	if _, err := q.LinkCustomerOrganization(ctx, db.LinkCustomerOrganizationParams{
		UserID: user.ID, OrganizationID: c.Org.InternalID, BrandID: c.Org.BrandID,
	}); err != nil {
		return db.User{}, false, nil, fmt.Errorf("customers: link: %w", err)
	}
	return user, existing, ignored, nil
}

// fillNames returns the name/surname arguments of a fill-only update and
// records submitted values that differ from a stored, non-empty one.
func fillNames(user db.User, name, surname string, ignored []string) ([]string, pgtype.Text, pgtype.Text) {
	var n, sn pgtype.Text
	if name != "" {
		if strings.TrimSpace(user.Name) == "" {
			n = text(name)
		} else if user.Name != name {
			ignored = append(ignored, "name")
		}
	}
	if surname != "" {
		if strings.TrimSpace(user.Surname) == "" {
			sn = text(surname)
		} else if user.Surname != surname {
			ignored = append(ignored, "surname")
		}
	}
	return ignored, n, sn
}

// writableUser refuses anonymized (K19) and inactive or merged accounts.
func writableUser(u db.User) error {
	switch {
	case u.Status == StatusAnonymized:
		return ErrAnonymized
	case u.Status != StatusActive || u.MergedIntoUserID.Valid:
		return ErrInactive
	}
	return nil
}

// applyProfile writes a profile patch. full=false is fill-only: a stored
// value is never replaced; differing submitted values are reported.
func (s *Service) applyProfile(ctx context.Context, q *db.Queries, userID int64, p profilePatch, full bool) ([]string, error) {
	var ignored []string
	prof, err := q.GetCustomerProfileForUpdate(ctx, userID)
	isNew := errors.Is(err, pgx.ErrNoRows)
	if err != nil && !isNew {
		return nil, fmt.Errorf("customers: profile: %w", err)
	}
	if isNew {
		prof = db.CustomerProfile{UserID: userID, Type: TypeIndividual, Address: []byte("{}")}
	}
	canReplace := full || isNew

	pick := func(field string, cur pgtype.Text, o Optional[string]) pgtype.Text {
		if !o.Set {
			return cur
		}
		nv := ""
		if o.Value != nil {
			nv = *o.Value
		}
		if canReplace || !cur.Valid || cur.String == "" {
			return text(nv)
		}
		if nv != "" && nv != cur.String {
			ignored = append(ignored, field)
		}
		return cur
	}
	typ := prof.Type
	if p.Type.Set && p.Type.Value != nil && *p.Type.Value != "" && *p.Type.Value != typ {
		if canReplace {
			typ = *p.Type.Value
		} else {
			ignored = append(ignored, "type")
		}
	}
	company := pick("company_name", prof.CompanyName, p.CompanyName)
	taxOffice := pick("tax_office", prof.TaxOffice, p.TaxOffice)
	address := prof.Address
	if p.Address.Set {
		nv := *p.Address.Value
		switch {
		case canReplace || isEmptyAddress(prof.Address):
			address = nv
		case !jsonEqual(nv, prof.Address) && !isEmptyAddress(nv):
			ignored = append(ignored, "address")
		}
	}
	prefs := prof.NotificationPrefs
	if p.Prefs.Set {
		merged, err := mergeNotificationPrefs(prof.NotificationPrefs, *p.Prefs.Value)
		if err != nil {
			return nil, err
		}
		switch {
		case canReplace:
			prefs = merged
		case !jsonEqual(merged, prof.NotificationPrefs) && prof.NotificationPrefs != nil:
			// Preferences belong to the customer; another organization's
			// request does not change them.
			ignored = append(ignored, "notification_prefs")
		}
	}

	if isNew {
		if prefs == nil {
			if prefs, err = mergeNotificationPrefs(nil, nil); err != nil {
				return nil, err
			}
		}
		if _, err := q.CreateCustomerProfile(ctx, db.CreateCustomerProfileParams{
			UserID: userID, Type: typ, CompanyName: company, TaxOffice: taxOffice,
			Address: address, NotificationPrefs: prefs,
		}); err != nil {
			return nil, fmt.Errorf("customers: create profile: %w", err)
		}
	} else if _, err := q.UpdateCustomerProfile(ctx, db.UpdateCustomerProfileParams{
		UserID: userID, Type: typ, CompanyName: company, TaxOffice: taxOffice,
		Address: address, NotificationPrefs: prefs,
	}); err != nil {
		return nil, fmt.Errorf("customers: update profile: %w", err)
	}

	// Identity numbers: sealed with PIIBox, only last4 is ever read back.
	setNational := func(enc []byte, last4 pgtype.Text) error {
		_, err := q.SetCustomerNationalID(ctx, db.SetCustomerNationalIDParams{UserID: userID, NationalIDEnc: enc, NationalIDLast4: last4})
		return err
	}
	setTax := func(enc []byte, last4 pgtype.Text) error {
		_, err := q.SetCustomerTaxNo(ctx, db.SetCustomerTaxNoParams{UserID: userID, TaxNoEnc: enc, TaxNoLast4: last4})
		return err
	}
	for _, f := range []struct {
		field string
		aad   string
		cur   []byte
		o     Optional[string]
		set   func([]byte, pgtype.Text) error
	}{
		{"national_id", aadNationalID, prof.NationalIDEnc, p.NationalID, setNational},
		{"tax_no", aadTaxNo, prof.TaxNoEnc, p.TaxNo, setTax},
	} {
		if !f.o.Set {
			continue
		}
		nv := ""
		if f.o.Value != nil {
			nv = *f.o.Value
		}
		if !canReplace && len(f.cur) > 0 {
			if nv != "" && !s.sealedEquals(f.cur, f.aad, userID, nv) {
				ignored = append(ignored, f.field)
			}
			continue
		}
		if nv == "" {
			if len(f.cur) > 0 {
				if err := f.set(nil, pgtype.Text{}); err != nil {
					return nil, fmt.Errorf("customers: clear %s: %w", f.field, err)
				}
			}
			continue
		}
		if s.pii == nil {
			return nil, ErrPIIUnavailable
		}
		enc, err := s.pii.Seal(nv, crypto.PIIAAD(f.aad, userID))
		if err != nil {
			return nil, fmt.Errorf("customers: seal %s: %w", f.field, err)
		}
		if err := f.set(enc, text(crypto.Last4(nv))); err != nil {
			return nil, fmt.Errorf("customers: set %s: %w", f.field, err)
		}
	}
	return ignored, nil
}

// sealedEquals reports whether a stored identity number equals v (used only
// to avoid reporting an identical value as ignored; never returned).
func (s *Service) sealedEquals(sealed []byte, aad string, userID int64, v string) bool {
	if s.pii == nil {
		return false
	}
	plain, err := s.pii.Open(sealed, crypto.PIIAAD(aad, userID))
	return err == nil && plain == v
}

func jsonEqual(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return bytes.Equal(a, b)
	}
	ax, _ := json.Marshal(x)
	by, _ := json.Marshal(y)
	return bytes.Equal(ax, by)
}

// --- Read --------------------------------------------------------------------

// ListCustomers lists the customers linked to the organizations in scope.
func (s *Service) ListCustomers(ctx context.Context, c Caller, f ListFilter) ([]CustomerSummary, int64, error) {
	status := strings.TrimSpace(f.Status)
	switch status {
	case "", "active", "disabled", "pending", StatusAnonymized:
	default:
		return nil, 0, invalid("status", "must be active, disabled, pending or anonymized")
	}
	q := strings.TrimSpace(f.Q)
	if len(q) > 100 {
		return nil, 0, invalid("q", "must be at most 100 characters")
	}
	var (
		rows    []db.ListOrganizationCustomersRow
		total   int64
		indexed bool
	)
	// TEC-164: a text search goes to the customers index when it is up
	// (anonymized customers are not indexed, so that status stays on SQL).
	if q != "" && status != StatusAnonymized && s.indexEnabled() {
		rows, total, indexed = s.searchIndexed(ctx, c, status, q, f.Limit, f.Offset)
	}
	if !indexed {
		var err error
		rows, err = s.q.ListOrganizationCustomers(ctx, db.ListOrganizationCustomersParams{
			OrgIds: c.orgIDs(), BrandID: c.brand(), Status: text(status), Q: text(q),
			LimitCount: f.Limit, OffsetCount: f.Offset,
		})
		if err != nil {
			return nil, 0, fmt.Errorf("customers: list: %w", err)
		}
		total, err = s.q.CountOrganizationCustomers(ctx, db.CountOrganizationCustomersParams{
			OrgIds: c.orgIDs(), BrandID: c.brand(), Status: text(status), Q: text(q),
		})
		if err != nil {
			return nil, 0, fmt.Errorf("customers: count: %w", err)
		}
	}
	out := make([]CustomerSummary, 0, len(rows))
	for _, r := range rows {
		sum := CustomerSummary{
			UUID: r.Uuid, Name: r.Name, Surname: r.Surname, Email: strOrNil(r.Email), Phone: strOrNil(r.PhoneE164),
			Status: r.Status, Type: TypeIndividual, CompanyName: strOrNil(r.CompanyName), Locale: strOrNil(r.Locale),
			CreatedAt: r.CreatedAt.Time, LinkedAt: timePtr(r.LinkedAt), FirstServiceAt: timePtr(r.FirstServiceAt),
		}
		if r.CustomerType.Valid {
			sum.Type = r.CustomerType.String
		}
		out = append(out, maskSummary(sum, c.Locale))
	}
	return out, total, nil
}

// GetCustomer returns one customer of the scope (404 outside it).
func (s *Service) GetCustomer(ctx context.Context, c Caller, id uuid.UUID) (CustomerDetail, error) {
	user, err := s.scopedUser(ctx, s.q, c, id)
	if err != nil {
		return CustomerDetail{}, err
	}
	return s.detail(ctx, s.q, c, user)
}

// scopedUser loads a customer linked to the caller's scope.
func (s *Service) scopedUser(ctx context.Context, q *db.Queries, c Caller, id uuid.UUID) (db.User, error) {
	user, err := q.GetUserByUUID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.User{}, ErrCustomerNotFound
	}
	if err != nil {
		return db.User{}, fmt.Errorf("customers: user: %w", err)
	}
	if err := s.requireInScope(ctx, q, c, user.ID); err != nil {
		return db.User{}, err
	}
	return user, nil
}

func (s *Service) requireInScope(ctx context.Context, q *db.Queries, c Caller, userID int64) error {
	if c.Org.BrandID == 0 {
		return ErrCustomerNotFound
	}
	ok, err := q.CustomerInScope(ctx, db.CustomerInScopeParams{UserID: userID, OrgIds: c.orgIDs(), BrandID: c.brand()})
	if err != nil {
		return fmt.Errorf("customers: scope: %w", err)
	}
	if !ok {
		return ErrCustomerNotFound
	}
	return nil
}

func (s *Service) detail(ctx context.Context, q *db.Queries, c Caller, user db.User) (CustomerDetail, error) {
	d := CustomerDetail{
		CustomerSummary: CustomerSummary{
			UUID: user.Uuid, Name: user.Name, Surname: user.Surname, Email: strOrNil(user.Email),
			Phone: strOrNil(user.PhoneE164), Status: user.Status, Type: TypeIndividual,
			Locale: strOrNil(user.Locale), CreatedAt: user.CreatedAt.Time,
		},
		Address:           json.RawMessage("{}"),
		NotificationPrefs: json.RawMessage("{}"),
		Organizations:     []OrganizationLink{},
	}
	prof, err := q.GetCustomerProfile(ctx, user.ID)
	switch {
	case err == nil:
		d.Type = prof.Type
		d.CompanyName = strOrNil(prof.CompanyName)
		d.TaxOffice = strOrNil(prof.TaxOffice)
		d.NationalIDLast4 = strOrNil(prof.NationalIDLast4)
		d.TaxNoLast4 = strOrNil(prof.TaxNoLast4)
		if len(prof.Address) > 0 {
			d.Address = json.RawMessage(prof.Address)
		}
		if len(prof.NotificationPrefs) > 0 {
			d.NotificationPrefs = json.RawMessage(prof.NotificationPrefs)
		}
	case !errors.Is(err, pgx.ErrNoRows):
		return CustomerDetail{}, fmt.Errorf("customers: profile: %w", err)
	}
	links, err := q.ListCustomerOrganizationsByUser(ctx, db.ListCustomerOrganizationsByUserParams{
		UserID: user.ID, OrgIds: c.orgIDs(), BrandID: c.brand(),
	})
	if err != nil {
		return CustomerDetail{}, fmt.Errorf("customers: links: %w", err)
	}
	for _, l := range links {
		d.Organizations = append(d.Organizations, OrganizationLink{
			UUID: l.OrganizationUuid, Name: l.OrganizationName, Type: l.OrganizationType,
			LinkedAt: l.CreatedAt.Time, FirstServiceAt: timePtr(l.FirstServiceAt),
		})
		if d.LinkedAt == nil || l.CreatedAt.Time.Before(*d.LinkedAt) {
			t := l.CreatedAt.Time
			d.LinkedAt = &t
		}
		if l.FirstServiceAt.Valid && (d.FirstServiceAt == nil || l.FirstServiceAt.Time.Before(*d.FirstServiceAt)) {
			t := l.FirstServiceAt.Time
			d.FirstServiceAt = &t
		}
	}
	d.Editable = writableUser(user) == nil
	if d.Editable {
		full, err := s.identityEditable(ctx, q, c, user.ID)
		if err != nil {
			return CustomerDetail{}, err
		}
		d.IdentityEditable = full
	}
	if user.Status == StatusAnonymized {
		d.CustomerSummary = maskSummary(d.CustomerSummary, c.Locale)
		d.TaxOffice, d.NationalIDLast4, d.TaxNoLast4 = nil, nil, nil
		d.Address = json.RawMessage("{}")
	}
	return d, nil
}

// identityEditable: the caller may replace (not only fill) the customer's
// name, e-mail and profile when every organization link of the customer is
// inside its scope and the user is not a panel account (no membership, no
// staff role). Otherwise another organization's data would be overwritten.
func (s *Service) identityEditable(ctx context.Context, q *db.Queries, c Caller, userID int64) (bool, error) {
	links, err := q.CountCustomerOrganizationLinks(ctx, db.CountCustomerOrganizationLinksParams{
		UserID: userID, OrgIds: c.orgIDs(), BrandID: c.brand(),
	})
	if err != nil {
		return false, fmt.Errorf("customers: links: %w", err)
	}
	if links.Total == 0 || links.InScope != links.Total {
		return false, nil
	}
	members, err := q.CountOrganizationMembershipsByUser(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("customers: memberships: %w", err)
	}
	if members > 0 {
		return false, nil
	}
	roles, err := q.ListUserRoleSlugs(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("customers: roles: %w", err)
	}
	for _, r := range roles {
		if r != rbac.RoleCustomer && r != rbac.RoleFleet {
			return false, nil
		}
	}
	return true, nil
}

// maskSummary hides the personal data of an anonymized customer (K19).
func maskSummary(s CustomerSummary, loc i18n.Locale) CustomerSummary {
	if s.Status != StatusAnonymized {
		return s
	}
	s.Anonymized = true
	s.Name = i18n.Translate(loc, AnonymizedNameKey)
	s.Surname = ""
	s.Email, s.Phone, s.CompanyName = nil, nil, nil
	return s
}

// --- Update ------------------------------------------------------------------

// UpdateCustomer edits a customer of the scope. Anonymized customers are
// read-only. Name, e-mail and profile are replaced only when the caller owns
// every link of the customer (identity_editable); otherwise only empty
// fields are filled and the rest is reported in IgnoredFields.
func (s *Service) UpdateCustomer(ctx context.Context, c Caller, id uuid.UUID, in UpdateCustomerInput) (CustomerWrite, error) {
	name, surname, email := in.Name, in.Surname, in.Email
	var err error
	normalize := func(o Optional[string], fn func(string) (string, error)) (Optional[string], error) {
		if !o.Set {
			return o, nil
		}
		raw := ""
		if o.Value != nil {
			raw = *o.Value
		}
		v, err := fn(raw)
		if err != nil {
			return o, err
		}
		return Of(v), nil
	}
	if name, err = normalize(name, func(v string) (string, error) { return normalizeName(v, "name") }); err != nil {
		return CustomerWrite{}, err
	}
	if name.Set && *name.Value == "" {
		return CustomerWrite{}, invalid("name", "is required")
	}
	if surname, err = normalize(surname, func(v string) (string, error) { return normalizeName(v, "surname") }); err != nil {
		return CustomerWrite{}, err
	}
	if email, err = normalize(email, normalizeEmail); err != nil {
		return CustomerWrite{}, err
	}
	patch, err := normalizeProfilePatch(in.Type, in.CompanyName, in.TaxOffice, in.NationalID, in.TaxNo, in.Address, in.NotificationPrefs)
	if err != nil {
		return CustomerWrite{}, err
	}
	if patch.needsPII() && s.pii == nil {
		return CustomerWrite{}, ErrPIIUnavailable
	}

	var (
		user    db.User
		ignored []string
	)
	err = s.inTx(ctx, func(q *db.Queries) error {
		var err error
		if user, err = s.scopedUser(ctx, q, c, id); err != nil {
			return err
		}
		if err := writableUser(user); err != nil {
			return err
		}
		full, err := s.identityEditable(ctx, q, c, user.ID)
		if err != nil {
			return err
		}
		if email.Set && *email.Value != "" && (!user.Email.Valid || user.Email.String != *email.Value) {
			other, err := q.GetUserByEmail(ctx, text(*email.Value))
			switch {
			case err == nil && other.ID != user.ID:
				if full {
					return ErrEmailTaken
				}
				ignored = append(ignored, "email")
				email = Optional[string]{}
			case err != nil && !errors.Is(err, pgx.ErrNoRows):
				return fmt.Errorf("customers: email lookup: %w", err)
			}
		}
		if full {
			p := db.SetCustomerIdentityParams{ID: user.ID, Name: user.Name, Surname: user.Surname, Email: user.Email}
			if name.Set {
				p.Name = *name.Value
			}
			if surname.Set {
				p.Surname = *surname.Value
			}
			if email.Set {
				p.Email = text(*email.Value)
				if !p.Email.Valid && !user.PhoneE164.Valid {
					return invalid("email", "is required for a customer without a phone")
				}
			}
			if user, err = q.SetCustomerIdentity(ctx, p); err != nil {
				if isUniqueViolation(err, "uq_users_email_active") {
					return ErrEmailTaken
				}
				return err
			}
		} else {
			fill := db.FillCustomerIdentityParams{ID: user.ID}
			n, sn := "", ""
			if name.Set {
				n = *name.Value
			}
			if surname.Set {
				sn = *surname.Value
			}
			ignored, fill.Name, fill.Surname = fillNames(user, n, sn, ignored)
			if email.Set && *email.Value != "" {
				if user.Email.Valid {
					if user.Email.String != *email.Value {
						ignored = append(ignored, "email")
					}
				} else {
					fill.Email = text(*email.Value)
				}
			}
			if user, err = q.FillCustomerIdentity(ctx, fill); err != nil {
				if isUniqueViolation(err, "uq_users_email_active") {
					return ErrEmailTaken
				}
				return err
			}
		}
		profIgnored, err := s.applyProfile(ctx, q, user.ID, patch, full)
		if err != nil {
			return err
		}
		ignored = append(ignored, profIgnored...)
		return nil
	})
	if err != nil {
		return CustomerWrite{}, err
	}
	s.indexCustomer(ctx, user.Uuid) // TEC-164
	detail, err := s.detail(ctx, s.q, c, user)
	if err != nil {
		return CustomerWrite{}, err
	}
	if ignored == nil {
		ignored = []string{}
	}
	return CustomerWrite{CustomerDetail: detail, ExistingUser: true, IgnoredFields: ignored}, nil
}

// --- Upgrade to dealer -------------------------------------------------------

// UpgradeInput is POST /v1/customers/{uuid}/upgrade-to-dealer.
type UpgradeInput struct {
	OrganizationUUID uuid.UUID `json:"organization_uuid"`
	Role             string    `json:"role"`
}

// UpgradeResult describes the new membership.
type UpgradeResult struct {
	CustomerUUID uuid.UUID `json:"customer_uuid"`
	Organization struct {
		UUID uuid.UUID `json:"uuid"`
		Slug string    `json:"slug"`
		Name string    `json:"name"`
		Type string    `json:"type"`
	} `json:"organization"`
	Role       string `json:"role"`
	MemberRole string `json:"member_role"`
}

// UpgradeToDealer makes a customer a member of an existing dealer
// (TEC-100 decision 5: no new organization). Only the center or a
// distributor above the dealer may do it. The customer role, links,
// vehicles and history stay; the same user then signs in to the portal
// (customer role) and to the panel (membership).
func (s *Service) UpgradeToDealer(ctx context.Context, c Caller, id uuid.UUID, in UpgradeInput) (UpgradeResult, error) {
	switch c.Org.OrgType {
	case rbac.OrgTypeCenter, rbac.OrgTypeDistributor:
	default:
		return UpgradeResult{}, ErrForbidden
	}
	var memberRole string
	switch in.Role {
	case rbac.RoleDealerOwner:
		memberRole = "owner"
	case rbac.RoleDealerStaff:
		memberRole = "staff"
	default:
		return UpgradeResult{}, invalid("role", "must be dealer_owner or dealer_staff")
	}
	if in.OrganizationUUID == uuid.Nil {
		return UpgradeResult{}, invalid("organization_uuid", "is required")
	}
	var res UpgradeResult
	err := s.inTx(ctx, func(q *db.Queries) error {
		user, err := s.scopedUser(ctx, q, c, id)
		if err != nil {
			return err
		}
		if err := writableUser(user); err != nil {
			return err
		}
		org, err := q.GetOrganizationByUUID(ctx, in.OrganizationUUID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOrganizationNotFound
		}
		if err != nil {
			return fmt.Errorf("customers: organization: %w", err)
		}
		// Brand of the domain, inside the caller's reach (center: brand,
		// distributor: its subtree) and strictly below the caller.
		if org.DeletedAt.Valid || org.BrandID != c.Org.BrandID || org.ID == c.Org.InternalID ||
			!c.Filter.AllowsOrg(org.ID, org.BrandID) {
			return ErrOrganizationNotFound
		}
		if org.Type != rbac.OrgTypeDealer {
			return invalid("organization_uuid", "must be a dealer")
		}
		if c.Org.OrgType == rbac.OrgTypeDistributor {
			below, err := q.Descendants(ctx, c.Org.InternalID)
			if err != nil {
				return fmt.Errorf("customers: descendants: %w", err)
			}
			found := false
			for _, o := range below {
				if o.ID == org.ID {
					found = true
					break
				}
			}
			if !found {
				return ErrOrganizationNotFound
			}
		}
		if _, err := q.GetOrganizationMember(ctx, db.GetOrganizationMemberParams{OrganizationID: org.ID, UserID: user.ID}); err == nil {
			return ErrAlreadyMember
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("customers: membership: %w", err)
		}
		member, err := q.CreateOrganizationMember(ctx, db.CreateOrganizationMemberParams{
			OrganizationID: org.ID, UserID: user.ID, Role: memberRole,
		})
		if err != nil {
			if isUniqueViolation(err, "uq_organization_members") {
				return ErrAlreadyMember
			}
			return fmt.Errorf("customers: membership: %w", err)
		}
		if err := q.AssignMemberRoleBySlug(ctx, db.AssignMemberRoleBySlugParams{MemberID: member.ID, Slug: in.Role}); err != nil {
			return fmt.Errorf("customers: member role: %w", err)
		}
		res.CustomerUUID = user.Uuid
		res.Organization.UUID, res.Organization.Slug = org.Uuid, org.Slug
		res.Organization.Name, res.Organization.Type = org.Name, org.Type
		res.Role, res.MemberRole = in.Role, memberRole
		return nil
	})
	if err != nil {
		return UpgradeResult{}, err
	}
	return res, nil
}

// --- helpers -----------------------------------------------------------------

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && (constraint == "" || pgErr.ConstraintName == constraint)
}

// unusablePasswordHash hashes 32 random bytes nobody knows: a customer
// created by an organization signs in with WhatsApp OTP (K11) and has no
// password until one is set explicitly.
func unusablePasswordHash() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return password.Hash(base64.RawStdEncoding.EncodeToString(b))
}

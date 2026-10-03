package migrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/normalize"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/crypto"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/password"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// CustomersStep imports hub customers (TEC-255, K11/K26/K29):
//
//   - every customer becomes a users row with the customer role, a
//     customer_profiles row and one customer_organizations row per dealer
//     that served it (the customer is global, K11);
//   - the phone is normalized to E.164 with the customer's country as the
//     default region (K29);
//   - live customers with the same E.164 share one user (K26): their
//     migration_map rows point at the same target uuid;
//   - a customer without a resolved phone is migrated "unverified"
//     (phone_verified_at NULL, users.legacy_unverified, migration 000078)
//     and is claimed by the first successful OTP on its phone.
//
// A merged or changed customer only fills what the account lacks, so a
// later source never overwrites an earlier one or an edit in the new app.
// It runs after OrganizationsStep (dealer organizations).
type CustomersStep struct {
	// System is the migration_map source system; empty means SourceHub.
	System string
	// PII seals national id / tax numbers. Nil reads CUSTOMER_PII_KEY; when
	// that is empty too the numbers are skipped and counted.
	PII *crypto.PIIBox
}

// Name implements Step.
func (CustomersStep) Name() string { return "customers" }

func (s CustomersStep) system() string {
	if s.System == "" {
		return SourceHub
	}
	return s.System
}

const customersQuery = `SELECT id, dealer_id, type, COALESCE(tc_no, ''), COALESCE(tax_no, ''), COALESCE(tax_office, ''),
	name, COALESCE(phone, ''), COALESCE(email, ''), COALESCE(address, ''), COALESCE(city, ''), COALESCE(district, ''),
	COALESCE(country, ''), notification_settings, created_at, updated_at, deleted_at
FROM customers`

type legacyCustomer struct {
	ID                              int64
	DealerID                        sql.NullInt64
	Type, NationalID, TaxNo, TaxOff string
	Name, Phone, Email, Address     string
	City, District, Country         string
	Notify                          []byte
	CreatedAt, UpdatedAt, DeletedAt sql.NullTime
}

// Customer profile types (chk_customer_profiles_type).
const (
	customerIndividual = "individual"
	customerCorporate  = "corporate"
)

// legacyIdentityRe mirrors the customers module rule for identity numbers.
var legacyIdentityRe = regexp.MustCompile(`^[0-9A-Z]{5,20}$`)

// Run implements Step.
func (s CustomersStep) Run(ctx context.Context, src Sources, dst *Target, m *Mapper) (StepResult, error) {
	c := counts{}
	hub, err := src.Get(SourceHub)
	if err != nil {
		return StepResult{}, err
	}
	pii := s.PII
	if pii == nil {
		if key := strings.TrimSpace(os.Getenv("CUSTOMER_PII_KEY")); key != "" {
			if pii, err = crypto.NewPIIBox(key); err != nil {
				return StepResult{}, err
			}
		}
	}
	tree, err := resolveOlexTree(ctx, dst.Q, false, c)
	if err != nil {
		return StepResult{Counts: c}, err
	}

	query, args := customersQuery, []any{}
	if dst.Mode == ModeDelta && !dst.Since.IsZero() {
		query += " WHERE COALESCE(updated_at, created_at) > ?"
		args = append(args, dst.Since)
	}
	rows, err := hub.Query(ctx, query+" ORDER BY id", args...)
	if err != nil {
		return StepResult{Counts: c}, err
	}
	var customers []legacyCustomer
	for rows.Next() {
		var lc legacyCustomer
		if err := rows.Scan(&lc.ID, &lc.DealerID, &lc.Type, &lc.NationalID, &lc.TaxNo, &lc.TaxOff, &lc.Name, &lc.Phone,
			&lc.Email, &lc.Address, &lc.City, &lc.District, &lc.Country, &lc.Notify,
			&lc.CreatedAt, &lc.UpdatedAt, &lc.DeletedAt); err != nil {
			_ = rows.Close()
			return StepResult{Counts: c}, fmt.Errorf("scan customer: %w", err)
		}
		customers = append(customers, lc)
	}
	if err := rows.Close(); err != nil {
		return StepResult{Counts: c}, fmt.Errorf("read customers: %w", err)
	}

	run := &customerRun{step: s, q: dst.Q, m: m, tree: tree, pii: pii, c: c,
		dealers: map[int64]int64{}, countries: map[string]int64{TRCountry: tree.CountryID}}
	var watermark time.Time
	for _, lc := range customers {
		c.inc(cntRead)
		if ts := latest(lc.CreatedAt, lc.UpdatedAt, lc.DeletedAt); ts.After(watermark) {
			watermark = ts
		}
		if err := run.importCustomer(ctx, lc); err != nil {
			return StepResult{Counts: c}, fmt.Errorf("customer %d: %w", lc.ID, err)
		}
	}
	return StepResult{Counts: c, Watermark: watermark}, nil
}

type customerRun struct {
	step      CustomersStep
	q         *db.Queries
	m         *Mapper
	tree      olexTree
	pii       *crypto.PIIBox
	c         counts
	dealers   map[int64]int64  // hub dealer id -> organization id (0: unmapped)
	countries map[string]int64 // ISO2 -> countries.id (0: unknown)
}

// customerData is a legacy customer after normalization.
type customerData struct {
	Email, Phone, PhoneRaw string
	Unverified             bool
	Name, Surname          string
	Type                   string
	Company, TaxOffice     pgtype.Text
	NationalID, TaxNo      string
	Address, Prefs         []byte
	Status                 string
}

// customerCountry is the ISO2 country of a legacy customer (TR default).
func customerCountry(raw string) string {
	iso := strings.ToUpper(strings.TrimSpace(raw))
	if len(iso) != 2 {
		return TRCountry
	}
	return iso
}

// normalizeCustomer maps the legacy values; it touches no database.
func normalizeCustomer(lc legacyCustomer) (customerData, []string) {
	var notes []string
	d := customerData{Status: "active", Type: customerIndividual}
	if lc.DeletedAt.Valid {
		d.Status = "disabled"
	}
	d.Email = truncate(normalize.NormalizeEmail(lc.Email), 255)

	ph := normalize.NormalizePhone(lc.Phone, customerCountry(lc.Country))
	if ph.Verified {
		d.Phone = ph.E164
	} else {
		// K26/K29: no phone or an unresolved one -> unverified, claimed later.
		d.Unverified = true
		d.PhoneRaw = truncate(ph.Raw, 64)
		if ph.Raw == "" {
			notes = append(notes, "phone_missing")
		} else {
			notes = append(notes, "phone_unresolved")
		}
	}

	switch strings.ToLower(strings.TrimSpace(lc.Type)) {
	case customerCorporate:
		d.Type = customerCorporate
	case customerIndividual, "":
	default:
		notes = append(notes, "type_unknown")
	}
	if d.Type == customerCorporate {
		d.Name = truncate(strings.TrimSpace(lc.Name), 100)
		if d.Name == "" {
			d.Name = "-"
		}
		d.Company = pgText(truncate(strings.TrimSpace(lc.Name), 200))
	} else {
		d.Name, d.Surname = splitName(lc.Name)
	}
	d.TaxOffice = pgText(truncate(strings.TrimSpace(lc.TaxOff), 150))

	var bad bool
	if d.NationalID, bad = identityNumber(lc.NationalID); bad {
		notes = append(notes, "national_id_invalid")
	}
	if d.TaxNo, bad = identityNumber(lc.TaxNo); bad {
		notes = append(notes, "tax_no_invalid")
	}

	d.Prefs = notificationPrefs(lc.Notify)
	return d, notes
}

// identityNumber cleans a TC / tax number like the customers module; bad is
// true when a non-empty value does not fit and is dropped.
func identityNumber(raw string) (string, bool) {
	var b strings.Builder
	for _, r := range strings.ToUpper(raw) {
		switch r {
		case ' ', '-', '.', '/', '\t':
			continue
		}
		b.WriteRune(r)
	}
	v := b.String()
	if v == "" {
		return "", false
	}
	if !legacyIdentityRe.MatchString(v) {
		return "", true
	}
	return v, false
}

// notificationPrefs lays the legacy {channel: bool} settings over the
// customer defaults; unknown channels and non-boolean values are ignored.
func notificationPrefs(raw []byte) []byte {
	prefs := map[string]bool{"whatsapp": true, "email": true, "sms": false, "push": true}
	var legacy map[string]any
	if len(raw) > 0 && json.Unmarshal(raw, &legacy) == nil {
		for k, v := range legacy {
			b, ok := v.(bool)
			if _, known := prefs[k]; known && ok {
				prefs[k] = b
			}
		}
	}
	out, _ := json.Marshal(prefs)
	return out
}

func (r *customerRun) importCustomer(ctx context.Context, lc legacyCustomer) error {
	d, notes := normalizeCustomer(lc)
	id := strconv.FormatInt(lc.ID, 10)
	for _, n := range notes {
		r.c.inc(n)
		if n != "phone_missing" {
			r.c.inc(n + ":customer:" + id)
		}
	}
	countryID, err := r.country(ctx, customerCountry(lc.Country))
	if err != nil {
		return err
	}
	if d.Address, err = r.address(ctx, countryID, lc, id); err != nil {
		return err
	}

	key := Key{System: r.step.system(), Table: "customers", ID: id, TargetTable: "users"}
	sum := Checksum(lc.DealerID.Int64, lc.DealerID.Valid, lc.Type, lc.NationalID, lc.TaxNo, lc.TaxOff, lc.Name, lc.Phone,
		lc.Email, lc.Address, lc.City, lc.District, lc.Country, string(lc.Notify), lc.DeletedAt.Time, lc.DeletedAt.Valid)

	_, mapped, err := r.m.Lookup(ctx, key.System, key.Table, key.ID)
	if err != nil {
		return err
	}
	if !mapped && d.Phone != "" && !lc.DeletedAt.Valid {
		// K26: a live account on the same E.164 (an earlier customer of this
		// run, a migrated user or one created in the new app) is the same
		// person: link instead of creating a second user.
		existing, err := r.q.MigratorFindUserByContact(ctx, db.MigratorFindUserByContactParams{PhoneE164: pgText(d.Phone)})
		switch {
		case err == nil:
			linked, err := r.m.Link(ctx, key, existing.Uuid, sum)
			if err != nil {
				return err
			}
			if linked {
				r.c.inc("merged")
				return r.attach(ctx, lc, d, existing.ID, true)
			}
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("find user by phone: %w", err)
		}
	}

	res, err := r.m.Upsert(ctx, key, sum)
	if err != nil {
		return err
	}
	current, err := r.q.MigratorUserByUUID(ctx, res.UUID)
	exists := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("read user: %w", err)
	}
	if exists {
		if !res.Changed {
			r.c.inc(cntUnchanged)
			return r.ensureLinks(ctx, lc, current.ID)
		}
		r.c.inc(cntUpdated)
		return r.attach(ctx, lc, d, current.ID, true)
	}

	// New user. Another live account may hold the e-mail (or, for a deleted
	// customer, nothing clashes: the unique indexes skip deleted rows).
	taken, err := r.q.MigratorContactTaken(ctx, db.MigratorContactTakenParams{
		Email: pgText(d.Email), PhoneE164: pgText(d.Phone),
	})
	if err != nil {
		return fmt.Errorf("contact check: %w", err)
	}
	if taken.EmailTaken && !lc.DeletedAt.Valid {
		r.c.inc("email_conflict:customer:" + id)
		d.Email = ""
	}
	if taken.PhoneTaken && !lc.DeletedAt.Valid {
		// Only reachable when the phone owner appeared after Lookup; keep
		// the raw value and treat the record as unverified.
		r.c.inc("phone_conflict:customer:" + id)
		d.PhoneRaw, d.Phone, d.Unverified = truncate(strings.TrimSpace(lc.Phone), 64), "", true
	}
	userID, err := r.q.MigratorInsertCustomerUser(ctx, db.MigratorInsertCustomerUserParams{
		Uuid: res.UUID, Email: pgText(d.Email), PasswordHash: password.ResetRequired,
		Name: d.Name, Surname: d.Surname, Status: d.Status, PhoneE164: pgText(d.Phone),
		LegacyUnverified: d.Unverified, LegacyPhoneRaw: pgText(d.PhoneRaw),
		CreatedAt: pgTime(lc.CreatedAt), DeletedAt: pgTime(lc.DeletedAt),
	})
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	r.c.inc(cntCreated)
	if d.Unverified {
		r.c.inc("unverified")
	}
	if lc.DeletedAt.Valid {
		r.c.inc("deleted")
	}
	return r.attach(ctx, lc, d, userID, false)
}

// attach gives the user the customer role, the profile (filling an existing
// one) and the organization link. fill is true for a merged or changed
// customer: the account's contact values are completed, never replaced.
func (r *customerRun) attach(ctx context.Context, lc legacyCustomer, d customerData, userID int64, fill bool) error {
	if fill {
		if err := r.fillUser(ctx, lc, d, userID); err != nil {
			return err
		}
	}

	n, err := r.q.MigratorAssignUserRole(ctx, db.MigratorAssignUserRoleParams{UserID: userID, Slug: rbac.RoleCustomer})
	if err != nil {
		return fmt.Errorf("grant customer role: %w", err)
	}
	r.c.add("user_roles_created", n)

	n, err = r.q.MigratorEnsureCustomerProfile(ctx, db.MigratorEnsureCustomerProfileParams{
		UserID: userID, Type: d.Type, CompanyName: d.Company, TaxOffice: d.TaxOffice,
		Address: d.Address, NotificationPrefs: d.Prefs, CreatedAt: pgTime(lc.CreatedAt),
	})
	if err != nil {
		return fmt.Errorf("profile: %w", err)
	}
	if n > 0 {
		r.c.inc("profiles_created")
	} else {
		n, err = r.q.MigratorFillCustomerProfile(ctx, db.MigratorFillCustomerProfileParams{
			UserID: userID, CompanyName: d.Company, TaxOffice: d.TaxOffice, Address: d.Address,
		})
		if err != nil {
			return fmt.Errorf("fill profile: %w", err)
		}
		r.c.add("profiles_filled", n)
	}
	if err := r.identityNumbers(ctx, userID, d, lc.ID); err != nil {
		return err
	}
	return r.ensureLinks(ctx, lc, userID)
}

// fillUser completes the e-mail / phone an account lacks; a value another
// live account holds is skipped and reported.
func (r *customerRun) fillUser(ctx context.Context, lc legacyCustomer, d customerData, userID int64) error {
	email, phone := d.Email, d.Phone
	if (email != "" || phone != "") && !lc.DeletedAt.Valid {
		taken, err := r.q.MigratorContactTaken(ctx, db.MigratorContactTakenParams{
			UserID: userID, Email: pgText(email), PhoneE164: pgText(phone),
		})
		if err != nil {
			return fmt.Errorf("contact check: %w", err)
		}
		id := strconv.FormatInt(lc.ID, 10)
		if taken.EmailTaken {
			r.c.inc("email_conflict:customer:" + id)
			email = ""
		}
		if taken.PhoneTaken {
			r.c.inc("phone_conflict:customer:" + id)
			phone = ""
		}
	}
	raw := ""
	if d.Unverified {
		raw = d.PhoneRaw
	}
	n, err := r.q.MigratorFillCustomerUser(ctx, db.MigratorFillCustomerUserParams{
		ID: userID, Email: pgText(email), PhoneE164: pgText(phone), LegacyPhoneRaw: pgText(raw),
	})
	if err != nil {
		return fmt.Errorf("fill user: %w", err)
	}
	r.c.add("users_filled", n)
	return nil
}

// identityNumbers seals the TC / tax number into an empty profile slot.
func (r *customerRun) identityNumbers(ctx context.Context, userID int64, d customerData, legacyID int64) error {
	if d.NationalID == "" && d.TaxNo == "" {
		return nil
	}
	if r.pii == nil {
		r.c.inc("pii_skipped")
		r.c.inc("pii_skipped:customer:" + strconv.FormatInt(legacyID, 10))
		return nil
	}
	prof, err := r.q.GetCustomerProfile(ctx, userID)
	if err != nil {
		return fmt.Errorf("read profile: %w", err)
	}
	if d.NationalID != "" && len(prof.NationalIDEnc) == 0 {
		enc, err := r.pii.Seal(d.NationalID, crypto.PIIAAD("customer_profiles.national_id", userID))
		if err != nil {
			return err
		}
		if _, err := r.q.SetCustomerNationalID(ctx, db.SetCustomerNationalIDParams{
			UserID: userID, NationalIDEnc: enc, NationalIDLast4: pgText(crypto.Last4(d.NationalID)),
		}); err != nil {
			return fmt.Errorf("set national id: %w", err)
		}
		r.c.inc("national_ids_set")
	}
	if d.TaxNo != "" && len(prof.TaxNoEnc) == 0 {
		enc, err := r.pii.Seal(d.TaxNo, crypto.PIIAAD("customer_profiles.tax_no", userID))
		if err != nil {
			return err
		}
		if _, err := r.q.SetCustomerTaxNo(ctx, db.SetCustomerTaxNoParams{
			UserID: userID, TaxNoEnc: enc, TaxNoLast4: pgText(crypto.Last4(d.TaxNo)),
		}); err != nil {
			return fmt.Errorf("set tax no: %w", err)
		}
		r.c.inc("tax_nos_set")
	}
	return nil
}

// ensureLinks links the customer to its dealer organization (the Olex
// center when the legacy row has no dealer or the dealer is not migrated).
func (r *customerRun) ensureLinks(ctx context.Context, lc legacyCustomer, userID int64) error {
	orgID := r.tree.CenterID
	if lc.DealerID.Valid {
		id, err := r.dealerOrg(ctx, lc.DealerID.Int64)
		if err != nil {
			return err
		}
		if id != 0 {
			orgID = id
		} else {
			r.c.inc("dealer_unmapped:customer:" + strconv.FormatInt(lc.ID, 10))
		}
	} else {
		r.c.inc("center_linked")
	}
	n, err := r.q.MigratorLinkCustomerOrganization(ctx, db.MigratorLinkCustomerOrganizationParams{
		UserID: userID, OrganizationID: orgID, BrandID: r.tree.BrandID, CreatedAt: pgTime(lc.CreatedAt),
	})
	if err != nil {
		return fmt.Errorf("link organization: %w", err)
	}
	r.c.add("links_created", n)
	return nil
}

// dealerOrg resolves a hub dealer to its organization (0: not migrated).
func (r *customerRun) dealerOrg(ctx context.Context, dealerID int64) (int64, error) {
	if id, ok := r.dealers[dealerID]; ok {
		return id, nil
	}
	target, ok, err := r.m.Lookup(ctx, r.step.system(), "dealers", strconv.FormatInt(dealerID, 10))
	if err != nil {
		return 0, err
	}
	id := int64(0)
	if ok {
		id, err = r.q.MigratorOrganizationIDByUUID(ctx, target)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("dealer organization: %w", err)
		}
	}
	r.dealers[dealerID] = id
	return id, nil
}

// country resolves an ISO2 code to countries.id (0 when unknown).
func (r *customerRun) country(ctx context.Context, iso string) (int64, error) {
	if id, ok := r.countries[iso]; ok {
		return id, nil
	}
	id := int64(0)
	row, err := r.q.GetCountryByISO2(ctx, iso)
	switch {
	case err == nil:
		id = row.ID
	case !errors.Is(err, pgx.ErrNoRows):
		return 0, fmt.Errorf("country %s: %w", iso, err)
	default:
		r.c.inc("country_unmatched:" + iso)
	}
	r.countries[iso] = id
	return id, nil
}

// address builds the profile address object: the legacy text plus the
// matched geo ids.
func (r *customerRun) address(ctx context.Context, countryID int64, lc legacyCustomer, id string) ([]byte, error) {
	addr := map[string]any{}
	put := func(k, v string) {
		if v = strings.TrimSpace(v); v != "" {
			addr[k] = v
		}
	}
	put("line", lc.Address)
	put("city", lc.City)
	put("district", lc.District)
	put("country", customerCountry(lc.Country))
	if countryID != 0 {
		addr["country_id"] = countryID
		province, district, err := matchAddress(ctx, r.q, countryID, lc.City, lc.District)
		if err != nil {
			return nil, err
		}
		if province.Valid {
			addr["province_id"] = province.Int64
		}
		if district.Valid {
			addr["district_id"] = district.Int64
		}
		if (strings.TrimSpace(lc.City) != "" && !province.Valid) || (strings.TrimSpace(lc.District) != "" && !district.Valid) {
			r.c.inc("geo_unmatched")
			r.c.inc("geo_unmatched:customer:" + id)
		}
	}
	return json.Marshal(addr)
}

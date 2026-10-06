package usecase

// TEC-379 (DT-BE-8): list contract of the accounting lists
// (docs/list-contract.md): accounts, cari, entries and disputes. The entry
// list and the entry export read the same parameters with
// ParseEntryFilter, so both select the same rows.

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// AccountSort is the sort contract of GET /v1/accounting/accounts. type
// sorts by type, then name (the old order).
var AccountSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"type": "type", "name": "name", "balance": "balance", "created_at": "created_at", "last_entry_at": "last_entry_at",
	},
	Default: apiquery.SortField{Field: "type"},
}

// CariSort is the sort contract of GET /v1/accounting/cari. name is the
// counterparty name.
var CariSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"name": "name", "balance": "balance", "entry_count": "entry_count",
		"last_entry_at": "last_entry_at", "created_at": "created_at",
	},
	Default: apiquery.SortField{Field: "name"},
}

// EntrySort is the sort contract of GET /v1/accounting/entries.
var EntrySort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"created_at": "created_at", "amount": "amount", "direction": "direction", "category": "category",
	},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

// DisputeSort is the sort contract of GET /v1/accounting/disputes. status
// sorts by rank (open → rejected), organization by the disputing
// organization name, amount by the disputed amount.
var DisputeSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"created_at": "created_at", "resolved_at": "resolved_at", "status": "status",
		"amount": "amount", "organization": "organization",
	},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

// Enum lists of the list filters.
var (
	AccountTypes = []string{AccountCash, AccountBank}
	// CariKinds: the counterparty organization type, or customer (a user).
	CariKinds        = []string{OrgCenter, OrgDistributor, OrgDealer, CariKindCustomer}
	EntryDirections  = []string{accounting.DirectionIncome, accounting.DirectionExpense, accounting.DirectionCharge, accounting.DirectionCollection, accounting.DirectionPayment, accounting.DirectionOpening}
	DisputeStatuses  = []string{DisputeOpen, DisputeResolvedReversal, DisputeResolvedRevision, DisputeRejected}
	sourceTypeFormat = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
)

// CariKindCustomer is the cari kind filter value of a customer (user) cari.
const CariKindCustomer = "customer"

// Entry list query keys (also the keys an entry export job carries).
const (
	QueryEntryAccountUUID = "account_uuid"
	QueryEntryDirection   = "direction"
	QueryEntryCategory    = "category"
	QueryEntrySourceType  = "source_type"
)

// entryListKeys are the entry list parameters an export job carries
// (cari_uuid is QueryCariUUID).
var entryListKeys = []string{
	"q", "sort", QueryEntryAccountUUID, QueryCariUUID, QueryEntryDirection, QueryEntryCategory,
	QueryEntrySourceType, "date_from", "date_to", "created_from", "created_to", "amount_min", "amount_max",
}

// EntryListValues keeps the entry list parameters of a job query (or an
// export body).
func EntryListValues(q map[string]string) url.Values {
	out := url.Values{}
	for _, k := range entryListKeys {
		if v := strings.TrimSpace(q[k]); v != "" {
			out.Set(k, v)
		}
	}
	return out
}

func listInvalid(field, msg string) error {
	return &apiquery.ValidationError{Details: []apiquery.Detail{{Field: field, Message: msg}}}
}

func optionalUUID(values url.Values, key string) (*uuid.UUID, error) {
	raw := strings.TrimSpace(values.Get(key))
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, listInvalid(key, "must be a UUID")
	}
	return &id, nil
}

func uuidList(values url.Values, key string) ([]uuid.UUID, error) {
	var out []uuid.UUID
	for _, raw := range apiquery.CSVValues(values, key) {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, listInvalid(key, "must be a list of UUIDs")
		}
		out = append(out, id)
	}
	return out, nil
}

// ParseAccountFilter reads organization_uuid, active, type (CSV), q and
// sort of GET /v1/accounting/accounts.
func ParseAccountFilter(values url.Values) (AccountFilter, error) {
	q := apiquery.Parse(values)
	f := AccountFilter{Q: q.Q}
	var err error
	if f.OrganizationUUID, err = optionalUUID(values, "organization_uuid"); err != nil {
		return f, err
	}
	if f.Active, err = apiquery.Bool(values, "active"); err != nil {
		return f, err
	}
	if f.Types, err = apiquery.EnumList(values, "type", AccountTypes...); err != nil {
		return f, err
	}
	f.Sort, err = apiquery.ResolveSort(q.Sort, AccountSort)
	return f, err
}

// ParseCariFilter reads organization_uuid, active, counterparty_kind (CSV),
// balance_min / balance_max, q, sort, limit and offset of
// GET /v1/accounting/cari.
func ParseCariFilter(values url.Values) (CariFilter, error) {
	q := apiquery.Parse(values)
	f := CariFilter{Q: q.Q, Limit: q.Limit, Offset: q.Offset}
	var err error
	if f.OrganizationUUID, err = optionalUUID(values, "organization_uuid"); err != nil {
		return f, err
	}
	if f.Active, err = apiquery.Bool(values, "active"); err != nil {
		return f, err
	}
	if f.Kinds, err = apiquery.EnumList(values, "counterparty_kind", CariKinds...); err != nil {
		return f, err
	}
	balance, err := apiquery.NumRange(values, "balance")
	if err != nil {
		return f, err
	}
	f.BalanceMin, f.BalanceMax = balance.Min, balance.Max
	f.Sort, err = apiquery.ResolveSort(q.Sort, CariSort)
	return f, err
}

// ParseEntryFilter reads organization_uuid, account_uuid, cari_uuid,
// direction / category / source_type (CSV), date_from / date_to or
// created_from / created_to, amount_min / amount_max, q, sort, limit and
// offset of GET /v1/accounting/entries.
func ParseEntryFilter(values url.Values) (EntryFilter, error) {
	q := apiquery.Parse(values)
	f := EntryFilter{Q: q.Q, Limit: q.Limit, Offset: q.Offset}
	var err error
	if f.OrganizationUUID, err = optionalUUID(values, "organization_uuid"); err != nil {
		return f, err
	}
	if f.AccountUUID, err = optionalUUID(values, QueryEntryAccountUUID); err != nil {
		return f, err
	}
	if f.CariUUID, err = optionalUUID(values, QueryCariUUID); err != nil {
		return f, err
	}
	if f.Directions, err = apiquery.EnumList(values, QueryEntryDirection, EntryDirections...); err != nil {
		return f, err
	}
	for _, c := range apiquery.CSVValues(values, QueryEntryCategory) {
		if !accounting.ValidCategoryKey(c) {
			return f, listInvalid(QueryEntryCategory, "is not a category key")
		}
		f.Categories = append(f.Categories, c)
	}
	for _, t := range apiquery.CSVValues(values, QueryEntrySourceType) {
		if !sourceTypeFormat.MatchString(t) {
			return f, listInvalid(QueryEntrySourceType, "is not a source type")
		}
		f.SourceTypes = append(f.SourceTypes, t)
	}
	date, err := apiquery.DateRange(values, "date")
	if err != nil {
		return f, err
	}
	created, err := apiquery.DateRange(values, "created")
	if err != nil {
		return f, err
	}
	if (date.From != nil && created.From != nil) || (date.Before != nil && created.Before != nil) {
		return f, listInvalid("created_from", "cannot be combined with date_from / date_to")
	}
	f.CreatedFrom, f.CreatedBefore = firstTime(created.From, date.From), firstTime(created.Before, date.Before)
	amount, err := apiquery.NumRange(values, "amount")
	if err != nil {
		return f, err
	}
	f.AmountMin, f.AmountMax = amount.Min, amount.Max
	f.Sort, err = apiquery.ResolveSort(q.Sort, EntrySort)
	return f, err
}

// ParseDisputeFilter reads status, organization_uuid and
// counterparty_organization_uuid (CSV), created_from / created_to, q, sort,
// limit and offset of GET /v1/accounting/disputes.
func ParseDisputeFilter(values url.Values) (DisputeFilter, error) {
	q := apiquery.Parse(values)
	f := DisputeFilter{Q: q.Q, Limit: q.Limit, Offset: q.Offset}
	var err error
	if f.Statuses, err = apiquery.EnumList(values, "status", DisputeStatuses...); err != nil {
		return f, err
	}
	if f.OrganizationUUIDs, err = uuidList(values, "organization_uuid"); err != nil {
		return f, err
	}
	if f.CounterpartyUUIDs, err = uuidList(values, "counterparty_organization_uuid"); err != nil {
		return f, err
	}
	created, err := apiquery.DateRange(values, "created")
	if err != nil {
		return f, err
	}
	f.CreatedFrom, f.CreatedBefore = created.From, created.Before
	f.Sort, err = apiquery.ResolveSort(q.Sort, DisputeSort)
	return f, err
}

func firstTime(a, b *time.Time) *time.Time {
	if a != nil {
		return a
	}
	return b
}

// escapeLike escapes the LIKE wildcards of a search term.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func likeArg(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: escapeLike(s), Valid: true}
}

func numArg(v *float64) pgtype.Numeric {
	var n pgtype.Numeric
	if v == nil {
		return n
	}
	if err := n.Scan(strconv.FormatFloat(*v, 'f', -1, 64)); err != nil {
		return pgtype.Numeric{}
	}
	return n
}

func timeArg(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// sortOrDefault applies the spec default when a filter was built by hand
// (no ParseXFilter) and carries no sort.
func sortOrDefault(s apiquery.ResolvedSort, spec apiquery.SortSpec) apiquery.ResolvedSort {
	if s.Key != "" {
		return s
	}
	r, _ := apiquery.ResolveSort(nil, spec)
	return r
}

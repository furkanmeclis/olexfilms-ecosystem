package usecase

// TEC-375 (DT-BE-6): list contract of the warehouse lists
// (docs/list-contract.md): stock entries, stock counts, warehouse transfers
// and end-of-day reports.

import (
	"net/url"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// EntrySort is the sort contract of GET /v1/warehouse/stock-entries. status
// sorts by flow rank (draft, confirmed, undone, cancelled); warehouse by the
// warehouse name, import entries (no warehouse) last.
var EntrySort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{"created_at": "created_at", "status": "status", "warehouse": "warehouse"},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

// CountSort is the sort contract of GET /v1/warehouse/stock-counts. status
// sorts by flow rank (draft, in_progress, pending_review, approved,
// cancelled); warehouse by the warehouse name.
var CountSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{"created_at": "created_at", "status": "status", "warehouse": "warehouse"},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

// TransferSort is the sort contract of GET /v1/warehouse/transfers. status
// sorts by flow rank (draft, in_transit, completed, cancelled).
var TransferSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{"transfer_no": "transfer_no", "created_at": "created_at", "status": "status"},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

// EODSort is the sort contract of GET /v1/warehouse/eod-reports. Within one
// key the system report comes before the warehouse reports.
var EODSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{"report_date": "report_date", "generated_at": "generated_at"},
	Default: apiquery.SortField{Field: "report_date", Desc: true},
}

// EntryListFilter is the parsed query of the stock entry list.
type EntryListFilter struct {
	Statuses       []string
	Modes          []string
	WarehouseUUIDs []uuid.UUID
	CreatedFrom    *time.Time
	CreatedBefore  *time.Time
	Q              string
	Sort           apiquery.ResolvedSort
	Limit, Offset  int32
}

// CountListFilter is the parsed query of the stock count list.
type CountListFilter struct {
	Statuses       []string
	Methods        []string
	Visibilities   []string
	ScopeTypes     []string
	WarehouseUUIDs []uuid.UUID
	CreatedFrom    *time.Time
	CreatedBefore  *time.Time
	Q              string
	Sort           apiquery.ResolvedSort
	Limit, Offset  int32
}

// TransferListFilter is the parsed query of the warehouse transfer list.
type TransferListFilter struct {
	Statuses           []string
	FromWarehouseUUIDs []uuid.UUID
	ToWarehouseUUIDs   []uuid.UUID
	CreatedFrom        *time.Time
	CreatedBefore      *time.Time
	Q                  string
	Sort               apiquery.ResolvedSort
	Limit, Offset      int32
}

// ParseEntryListFilter reads status, mode, warehouse_uuid (all CSV),
// created_from / created_to, q, sort, limit and offset. Errors are
// *apiquery.ValidationError (400).
func ParseEntryListFilter(values url.Values) (EntryListFilter, error) {
	q := apiquery.Parse(values)
	f := EntryListFilter{Q: q.Q, Limit: q.Limit, Offset: q.Offset}
	var err error
	if f.Statuses, err = apiquery.EnumList(values, "status",
		EntryStatusDraft, EntryStatusConfirmed, EntryStatusCancelled, EntryStatusUndone); err != nil {
		return f, err
	}
	if f.Modes, err = apiquery.EnumList(values, "mode", EntryModeWithExisting, EntryModeGenerateNew, EntryModeImport); err != nil {
		return f, err
	}
	if f.WarehouseUUIDs, err = uuidList(values, "warehouse_uuid"); err != nil {
		return f, err
	}
	if f.CreatedFrom, f.CreatedBefore, err = createdRange(values); err != nil {
		return f, err
	}
	f.Sort, err = apiquery.ResolveSort(q.Sort, EntrySort)
	return f, err
}

// ParseCountListFilter reads status, method, visibility, scope_type,
// warehouse_uuid (all CSV), created_from / created_to, q, sort, limit and
// offset.
func ParseCountListFilter(values url.Values) (CountListFilter, error) {
	q := apiquery.Parse(values)
	f := CountListFilter{Q: q.Q, Limit: q.Limit, Offset: q.Offset}
	var err error
	if f.Statuses, err = apiquery.EnumList(values, "status", CountStatusDraft, CountStatusInProgress,
		CountStatusPendingReview, CountStatusApproved, CountStatusCancelled); err != nil {
		return f, err
	}
	if f.Methods, err = apiquery.EnumList(values, "method", CountMethodLocationFirst, CountMethodUnitFirst,
		CountMethodProductQty, CountMethodInitialPlacement); err != nil {
		return f, err
	}
	if f.Visibilities, err = apiquery.EnumList(values, "visibility", CountVisibilityBlind, CountVisibilityGuided); err != nil {
		return f, err
	}
	if f.ScopeTypes, err = apiquery.EnumList(values, "scope_type",
		CountScopeWarehouse, CountScopeRoom, CountScopeLocation, CountScopeProduct); err != nil {
		return f, err
	}
	if f.WarehouseUUIDs, err = uuidList(values, "warehouse_uuid"); err != nil {
		return f, err
	}
	if f.CreatedFrom, f.CreatedBefore, err = createdRange(values); err != nil {
		return f, err
	}
	f.Sort, err = apiquery.ResolveSort(q.Sort, CountSort)
	return f, err
}

// ParseTransferListFilter reads status, from_warehouse_uuid,
// to_warehouse_uuid (all CSV), created_from / created_to, q, sort, limit and
// offset.
func ParseTransferListFilter(values url.Values) (TransferListFilter, error) {
	q := apiquery.Parse(values)
	f := TransferListFilter{Q: q.Q, Limit: q.Limit, Offset: q.Offset}
	var err error
	if f.Statuses, err = apiquery.EnumList(values, "status", TransferStatusDraft, TransferStatusInTransit,
		TransferStatusCompleted, TransferStatusCancelled); err != nil {
		return f, err
	}
	if f.FromWarehouseUUIDs, err = uuidList(values, "from_warehouse_uuid"); err != nil {
		return f, err
	}
	if f.ToWarehouseUUIDs, err = uuidList(values, "to_warehouse_uuid"); err != nil {
		return f, err
	}
	if f.CreatedFrom, f.CreatedBefore, err = createdRange(values); err != nil {
		return f, err
	}
	f.Sort, err = apiquery.ResolveSort(q.Sort, TransferSort)
	return f, err
}

// ParseEODListInput reads warehouse_uuid, scope, date_from, date_to, kind
// (CSV), sort, limit and offset.
func ParseEODListInput(values url.Values) (EODListInput, error) {
	q := apiquery.Parse(values)
	in := EODListInput{
		Scope:    strings.TrimSpace(values.Get("scope")),
		DateFrom: values.Get("date_from"), DateTo: values.Get("date_to"),
		Limit: q.Limit, Offset: q.Offset,
	}
	if raw := strings.TrimSpace(values.Get("warehouse_uuid")); raw != "" {
		in.WarehouseUUID = &raw
	}
	var err error
	if in.Kinds, err = apiquery.EnumList(values, "kind", EODKindAuto, EODKindManual); err != nil {
		return in, err
	}
	in.Sort, err = apiquery.ResolveSort(q.Sort, EODSort)
	return in, err
}

func uuidList(values url.Values, key string) ([]uuid.UUID, error) {
	raw := apiquery.CSVValues(values, key)
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, &apiquery.ValidationError{Details: []apiquery.Detail{{
				Field: key, Message: "must be a list of uuids", Code: "invalid",
			}}}
		}
		out = append(out, id)
	}
	return out, nil
}

func createdRange(values url.Values) (*time.Time, *time.Time, error) {
	r, err := apiquery.DateRange(values, "created")
	if err != nil {
		return nil, nil, err
	}
	return r.From, r.Before, nil
}

func listTS(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// listQ is the ILIKE argument of a q search (LIKE wildcards escaped); empty
// means no search.
func listQ(q string) pgtype.Text {
	q = strings.TrimSpace(q)
	if q == "" {
		return pgtype.Text{}
	}
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return pgtype.Text{String: r.Replace(q), Valid: true}
}

// orDefault returns the spec default when no sort was resolved (internal
// callers that build a filter by hand).
func orDefault(s apiquery.ResolvedSort, spec apiquery.SortSpec) apiquery.ResolvedSort {
	if s.Key == "" {
		return apiquery.ResolvedSort{Key: spec.Default.Field, Desc: spec.Default.Desc}
	}
	return s
}

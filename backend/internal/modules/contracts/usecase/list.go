package usecase

// TEC-1216: panel contract list GET /v1/contracts (docs/list-contract.md).
// It feeds the customer detail's contracts tab (customer_uuid) and a
// contracts screen; every relation is a refs.Ref, no internal id leaves.

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/refs"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ContractListSort is the sort contract of the panel contract list.
var ContractListSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{"created_at": "created_at", "contract_no": "contract_no", "status": "status"},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

// Contract list filter values.
var (
	ListStatuses = []string{model.StatusDraft, model.StatusPending, model.StatusExecuted, model.StatusVoided}
	ListKinds    = []string{model.KindVehicleIntake, model.KindServiceSale}
)

// ContractListFilter is the parsed GET /v1/contracts query.
type ContractListFilter struct {
	CustomerUUID pgtype.UUID
	Statuses     []string
	Kinds        []string
	Sort         apiquery.ResolvedSort
	Limit        int32
	Offset       int32
}

// ParseContractListFilter reads customer_uuid, status (CSV), kind (CSV), sort,
// limit and offset. Errors are *apiquery.ValidationError (400).
func ParseContractListFilter(values url.Values) (ContractListFilter, error) {
	q := apiquery.Parse(values)
	f := ContractListFilter{Limit: q.Limit, Offset: q.Offset}
	var err error
	if raw := strings.TrimSpace(values.Get("customer_uuid")); raw != "" {
		id, perr := uuid.Parse(raw)
		if perr != nil {
			return f, &apiquery.ValidationError{Details: []apiquery.Detail{{
				Field: "customer_uuid", Message: "must be a uuid", Code: "invalid",
			}}}
		}
		f.CustomerUUID = pgtype.UUID{Bytes: id, Valid: true}
	}
	if f.Statuses, err = apiquery.EnumList(values, "status", ListStatuses...); err != nil {
		return f, err
	}
	if f.Kinds, err = apiquery.EnumList(values, "kind", ListKinds...); err != nil {
		return f, err
	}
	if f.Sort, err = apiquery.ResolveSort(q.Sort, ContractListSort); err != nil {
		return f, err
	}
	return f, nil
}

// ContractListItem is one row of the panel contract list.
type ContractListItem struct {
	UUID         uuid.UUID `json:"uuid"`
	ContractNo   int64     `json:"contract_no"`
	Kind         string    `json:"kind"`
	Status       string    `json:"status"`
	SubjectType  string    `json:"subject_type"`
	Subject      *refs.Ref `json:"subject"`
	Customer     *refs.Ref `json:"customer"`
	Organization refs.Ref  `json:"organization"`
	// CustomerAnonymized: the customer is anonymized (K19); the name is
	// empty and clients show their anonymized label.
	CustomerAnonymized bool       `json:"customer_anonymized"`
	CreatedAt          time.Time  `json:"created_at"`
	ExecutedAt         *time.Time `json:"executed_at"`
	VoidedAt           *time.Time `json:"voided_at"`
}

// ListContracts returns the contracts inside the caller's contracts.read scope.
func (s *Service) ListContracts(ctx context.Context, c Caller, f ContractListFilter) ([]ContractListItem, int64, error) {
	sort := f.Sort
	if sort.Key == "" {
		sort = apiquery.ResolvedSort{Key: ContractListSort.Columns[ContractListSort.Default.Field], Desc: ContractListSort.Default.Desc}
	}
	p := db.ListContractInstancesPanelParams{
		BrandID: brandArg(c, c.Filter), OrgIds: orgIDsArg(c, c.Filter), CustomerUuid: f.CustomerUUID,
		Statuses: f.Statuses, Kinds: f.Kinds, SortKey: sort.Key, SortDesc: sort.Desc,
		RowLimit: f.Limit, RowOffset: f.Offset,
	}
	if c.Filter.UserOnly() {
		p.ActorUserID = pgtype.Int8{Int64: c.Filter.UserID, Valid: true}
	}
	q := s.repo.Queries()
	rows, err := q.ListContractInstancesPanel(ctx, p)
	if err != nil {
		return nil, 0, fmt.Errorf("contracts: list: %w", err)
	}
	total, err := q.CountContractInstancesPanel(ctx, db.CountContractInstancesPanelParams{
		BrandID: p.BrandID, OrgIds: p.OrgIds, ActorUserID: p.ActorUserID, CustomerUuid: p.CustomerUuid,
		Statuses: p.Statuses, Kinds: p.Kinds,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("contracts: count: %w", err)
	}
	out := make([]ContractListItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, contractListItem(r))
	}
	return out, total, nil
}

func contractListItem(r db.ListContractInstancesPanelRow) ContractListItem {
	it := ContractListItem{
		UUID: r.Uuid, ContractNo: r.ContractNo, Kind: r.Kind, Status: r.Status, SubjectType: r.SubjectType,
		Organization: refs.OrgRef(r.OrganizationUuid, r.OrganizationName, r.OrganizationType),
		CreatedAt:    r.CreatedAt.Time, ExecutedAt: tsPtr(r.ExecutedAt), VoidedAt: tsPtr(r.VoidedAt),
	}
	switch {
	case r.ServiceUuid.Valid:
		ref := refs.New(refs.KindService, r.ServiceUuid.Bytes, r.ServiceNo.String)
		it.Subject = &ref
	case r.SubscriptionUuid.Valid:
		ref := refs.New(refs.KindSubscription, r.SubscriptionUuid.Bytes, r.SubscriptionItemName.String)
		it.Subject = &ref
	}
	if r.CustomerUuid.Valid {
		name := refs.FullName(r.CustomerName.String, r.CustomerSurname.String)
		if r.CustomerStatus.String == "anonymized" {
			name, it.CustomerAnonymized = "", true
		}
		ref := refs.CustomerRef(r.CustomerUuid.Bytes, name)
		it.Customer = &ref
	}
	return it
}

func tsPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

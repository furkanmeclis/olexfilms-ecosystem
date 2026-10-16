package usecase

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-769 (Depo F4.4): user <-> warehouse membership.
//
// Rule: a user without any membership in the active organization works in
// every warehouse of it (backward compatible, the default). Once the user
// is a member of at least one warehouse, every warehouse write (stock
// entry, transfer, count, bin move, order shelving) needs a membership of
// the warehouse it touches; otherwise ErrNotWarehouseMember. Reads stay
// organization-wide: lists, scan, reports and exports are not filtered, so
// a picker can still show the stock of another warehouse (transfer source,
// center stock) and the read paths stay a single query each.

// ErrNotWarehouseMember: the caller has warehouse memberships in the
// organization and none of them is the warehouse the write touches.
var ErrNotWarehouseMember = errors.New("warehouse: user is not a member of this warehouse")

// ErrMemberNotFound: the user is not a member of the warehouse, or not a
// member of the organization.
var ErrMemberNotFound = errors.New("warehouse: member not found")

// Audit actions of the membership.
const (
	AuditResourceMember = "warehouse.member"
	AuditMemberAdded    = "warehouse.member.added"
	AuditMemberRemoved  = "warehouse.member.removed"
)

// MemberSort is the sort contract of GET /v1/warehouse/warehouses/{uuid}/members.
var MemberSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{"name": "name", "email": "email", "created_at": "created_at"},
	Default: apiquery.SortField{Field: "name"},
}

// MemberListFilter is the parsed member list query.
type MemberListFilter struct {
	Q      string
	Sort   apiquery.ResolvedSort
	Limit  int32
	Offset int32
}

// ParseMemberListFilter reads q, sort, limit and offset.
func ParseMemberListFilter(values url.Values) (MemberListFilter, error) {
	q := apiquery.Parse(values)
	f := MemberListFilter{Q: q.Q, Limit: q.Limit, Offset: q.Offset}
	var err error
	f.Sort, err = apiquery.ResolveSort(q.Sort, MemberSort)
	return f, err
}

// requireMember enforces the membership rule for one warehouse write. A
// call without a user (system jobs) is not restricted.
func requireMember(ctx context.Context, q *db.Queries, p authctx.Principal, org, warehouseID int64) error {
	if p.UserInternal <= 0 || warehouseID <= 0 {
		return nil
	}
	r, err := q.WarehouseMembershipCheck(ctx, db.WarehouseMembershipCheckParams{
		OrganizationID: org, UserID: p.UserInternal, WarehouseID: warehouseID,
	})
	if err != nil {
		return err
	}
	if r.HasAny && !r.IsMember {
		return ErrNotWarehouseMember
	}
	return nil
}

// requireAnyMember passes when the caller may write in one of the
// warehouses (a transfer touches its source and its target).
func requireAnyMember(ctx context.Context, q *db.Queries, p authctx.Principal, org int64, warehouseIDs ...int64) error {
	var last error
	for _, id := range warehouseIDs {
		err := requireMember(ctx, q, p, org, id)
		if err == nil {
			return nil
		}
		if !errors.Is(err, ErrNotWarehouseMember) {
			return err
		}
		last = err
	}
	return last
}

// defaultWarehouse returns the caller's default warehouse in the
// organization (0 when none).
func defaultWarehouse(ctx context.Context, q *db.Queries, p authctx.Principal, org int64) (int64, error) {
	if p.UserInternal <= 0 {
		return 0, nil
	}
	rows, err := q.ListUserWarehouseMemberships(ctx, db.ListUserWarehouseMembershipsParams{
		OrganizationID: org, UserID: p.UserInternal,
	})
	if err != nil {
		return 0, err
	}
	for _, r := range rows {
		if r.IsDefault {
			return r.WarehouseID, nil
		}
	}
	return 0, nil
}

// Members implements the membership use cases.
type Members struct {
	pool TxBeginner
	q    *db.Queries
}

// NewMembers builds the membership use case.
func NewMembers(pool TxBeginner, q *db.Queries) *Members {
	return &Members{pool: pool, q: q}
}

// MemberUser is the user of a membership.
type MemberUser struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   *string   `json:"email"`
}

// WarehouseMember is the API view of a membership.
type WarehouseMember struct {
	UUID      uuid.UUID  `json:"uuid"`
	User      MemberUser `json:"user"`
	IsDefault bool       `json:"is_default"`
	CreatedAt time.Time  `json:"created_at"`
}

// MemberInput adds a user to a warehouse.
type MemberInput struct {
	UserUUID  string
	IsDefault bool
}

// MyWarehouse is one warehouse of the caller's warehouse context.
type MyWarehouse struct {
	Warehouse
	// Member: the caller is a member (always true for an unrestricted
	// caller: every warehouse is writable).
	Member    bool `json:"member"`
	IsDefault bool `json:"is_default"`
}

// MyWarehouses is the warehouse context of the caller: restricted tells
// whether memberships limit the writes; DefaultUUID is the preselected
// warehouse (the member default; nil when none).
type MyWarehouses struct {
	Restricted  bool          `json:"restricted"`
	DefaultUUID *uuid.UUID    `json:"default_warehouse_uuid"`
	Items       []MyWarehouse `json:"items"`
}

func memberView(id uuid.UUID, isDefault bool, created pgtype.Timestamptz, uid uuid.UUID, name, surname string, email pgtype.Text) WarehouseMember {
	return WarehouseMember{
		UUID: id, IsDefault: isDefault, CreatedAt: ts(created),
		User: MemberUser{UUID: uid, Name: name, Surname: surname, Email: textPtr(email)},
	}
}

func (s *Members) warehouse(ctx context.Context, org int64, raw string) (db.Warehouse, error) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return db.Warehouse{}, ErrWarehouseNotFound
	}
	w, err := s.q.GetWarehouseByUUID(ctx, db.GetWarehouseByUUIDParams{Uuid: id, OrganizationID: org})
	return w, notFound(err, ErrWarehouseNotFound)
}

func (s *Members) orgUser(ctx context.Context, org int64, raw string) (db.GetOrganizationMemberUserRow, error) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return db.GetOrganizationMemberUserRow{}, invalid("user_uuid", "must be a uuid")
	}
	u, err := s.q.GetOrganizationMemberUser(ctx, db.GetOrganizationMemberUserParams{OrganizationID: org, UserUuid: id})
	return u, notFound(err, ErrMemberNotFound)
}

// List returns the members of a warehouse of the active organization.
func (s *Members) List(ctx context.Context, c Caller, warehouseUUID string, f MemberListFilter) ([]WarehouseMember, int64, error) {
	org, err := guard(c)
	if err != nil {
		return nil, 0, err
	}
	w, err := s.warehouse(ctx, org, warehouseUUID)
	if err != nil {
		return nil, 0, err
	}
	f.Sort = orDefault(f.Sort, MemberSort)
	if f.Limit <= 0 {
		f.Limit = apiquery.DefaultLimit
	}
	rows, err := s.q.ListWarehouseMembers(ctx, db.ListWarehouseMembersParams{
		OrganizationID: org, WarehouseID: w.ID, Q: listQ(f.Q),
		SortKey: f.Sort.Key, SortDesc: f.Sort.Desc, PageLimit: f.Limit, PageOffset: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountWarehouseMembers(ctx, db.CountWarehouseMembersParams{
		OrganizationID: org, WarehouseID: w.ID, Q: listQ(f.Q),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]WarehouseMember, 0, len(rows))
	for _, r := range rows {
		out = append(out, memberView(r.Uuid, r.IsDefault, r.CreatedAt, r.UserUuid, r.UserName, r.UserSurname, r.UserEmail))
	}
	return out, total, nil
}

// Add makes an organization member a member of the warehouse (idempotent).
// IsDefault also moves the user's default warehouse here.
func (s *Members) Add(ctx context.Context, c EntryCaller, warehouseUUID string, in MemberInput) (WarehouseMember, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return WarehouseMember{}, err
	}
	w, err := s.warehouse(ctx, org, warehouseUUID)
	if err != nil {
		return WarehouseMember{}, err
	}
	u, err := s.orgUser(ctx, org, in.UserUUID)
	if err != nil {
		return WarehouseMember{}, err
	}
	var row db.GetWarehouseMemberRow
	err = s.inTx(ctx, func(q *db.Queries) error {
		if in.IsDefault {
			if err := q.ClearWarehouseMemberDefault(ctx, db.ClearWarehouseMemberDefaultParams{OrganizationID: org, UserID: u.ID}); err != nil {
				return err
			}
		}
		m, err := q.InsertWarehouseMember(ctx, db.InsertWarehouseMemberParams{
			OrganizationID: org, WarehouseID: w.ID, UserID: u.ID, IsDefault: in.IsDefault, CreatedByUserID: c.actor(),
		})
		if err != nil {
			return mapDBError(err)
		}
		if row, err = q.GetWarehouseMember(ctx, db.GetWarehouseMemberParams{OrganizationID: org, WarehouseID: w.ID, UserID: u.ID}); err != nil {
			return err
		}
		return auditRow(ctx, q, c, AuditMemberAdded, AuditResourceMember, m.Uuid, map[string]any{
			"organization_id": org, "warehouse_id": w.ID, "user_id": u.ID, "is_default": m.IsDefault,
		})
	})
	if err != nil {
		return WarehouseMember{}, err
	}
	return memberView(row.Uuid, row.IsDefault, row.CreatedAt, row.UserUuid, row.UserName, row.UserSurname, row.UserEmail), nil
}

// Remove drops a user from a warehouse. Removing the last membership lifts
// the restriction (the user works in every warehouse again).
func (s *Members) Remove(ctx context.Context, c EntryCaller, warehouseUUID, userUUID string) error {
	org, err := guard(c.Caller)
	if err != nil {
		return err
	}
	w, err := s.warehouse(ctx, org, warehouseUUID)
	if err != nil {
		return err
	}
	uid, err := uuid.Parse(strings.TrimSpace(userUUID))
	if err != nil {
		return ErrMemberNotFound
	}
	return s.inTx(ctx, func(q *db.Queries) error {
		u, err := q.GetOrganizationMemberUser(ctx, db.GetOrganizationMemberUserParams{OrganizationID: org, UserUuid: uid})
		if err != nil {
			return notFound(err, ErrMemberNotFound)
		}
		m, err := q.GetWarehouseMember(ctx, db.GetWarehouseMemberParams{OrganizationID: org, WarehouseID: w.ID, UserID: u.ID})
		if err != nil {
			return notFound(err, ErrMemberNotFound)
		}
		if _, err := q.DeleteWarehouseMember(ctx, db.DeleteWarehouseMemberParams{OrganizationID: org, WarehouseID: w.ID, UserID: u.ID}); err != nil {
			return err
		}
		return auditRow(ctx, q, c, AuditMemberRemoved, AuditResourceMember, m.Uuid, map[string]any{
			"organization_id": org, "warehouse_id": w.ID, "user_id": u.ID,
		})
	})
}

// Mine returns the caller's warehouse context: every active warehouse of
// the organization with the member / default flags.
func (s *Members) Mine(ctx context.Context, c EntryCaller) (MyWarehouses, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return MyWarehouses{}, err
	}
	whs, err := s.q.ListWarehouses(ctx, db.ListWarehousesParams{OrganizationID: org, Active: pgtype.Bool{Bool: true, Valid: true}})
	if err != nil {
		return MyWarehouses{}, err
	}
	var rows []db.ListUserWarehouseMembershipsRow
	if c.Principal.UserInternal > 0 {
		if rows, err = s.q.ListUserWarehouseMemberships(ctx, db.ListUserWarehouseMembershipsParams{
			OrganizationID: org, UserID: c.Principal.UserInternal,
		}); err != nil {
			return MyWarehouses{}, err
		}
	}
	member := make(map[int64]bool, len(rows))
	var def int64
	for _, r := range rows {
		member[r.WarehouseID] = true
		if r.IsDefault {
			def = r.WarehouseID
		}
	}
	out := MyWarehouses{Restricted: len(rows) > 0, Items: make([]MyWarehouse, 0, len(whs))}
	for _, w := range whs {
		item := MyWarehouse{Warehouse: warehouseView(w), Member: !out.Restricted || member[w.ID], IsDefault: w.ID == def}
		if item.IsDefault {
			id := w.Uuid
			out.DefaultUUID = &id
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

// SetDefault moves the caller's own default warehouse; the caller must be
// a member of it (an unrestricted caller keeps the preference locally).
func (s *Members) SetDefault(ctx context.Context, c EntryCaller, warehouseUUID string) (MyWarehouses, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return MyWarehouses{}, err
	}
	w, err := s.warehouse(ctx, org, warehouseUUID)
	if err != nil {
		return MyWarehouses{}, err
	}
	err = s.inTx(ctx, func(q *db.Queries) error {
		if err := q.ClearWarehouseMemberDefault(ctx, db.ClearWarehouseMemberDefaultParams{OrganizationID: org, UserID: c.Principal.UserInternal}); err != nil {
			return err
		}
		n, err := q.SetWarehouseMemberDefault(ctx, db.SetWarehouseMemberDefaultParams{
			OrganizationID: org, UserID: c.Principal.UserInternal, WarehouseID: w.ID,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotWarehouseMember
		}
		return nil
	})
	if err != nil {
		return MyWarehouses{}, err
	}
	return s.Mine(ctx, c)
}

func (s *Members) inTx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrMemberNotFound
		}
		return err
	}
	return tx.Commit(ctx)
}

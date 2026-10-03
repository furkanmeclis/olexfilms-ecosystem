package migrator

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/source"
	warranty "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
)

// WarrantiesStep imports the hub warranties and the completed vehicle
// transfers (TEC-260, design §7 "Garanti: çift kayıtlar birleştirilir"):
//
//   - Duplicates: the hub opened a warranty per (service, stock item) on
//     every completion, so a service completed twice has two. The rule is
//     fixed here: the warranties of the same service item (the item kind is
//     always full, so the type is the same) become one warranty, the one
//     with the earliest start date (then the lowest id). Its end is the
//     latest end of the group and it is active when any of them is. Every
//     legacy id of the group maps to that warranty in migration_map.
//   - A full unit has one active warranty per vehicle
//     (uq_warranties_active_full_unit): a legacy warranty of the same
//     vehicle and unit under another service is a duplicate too and is
//     merged into the earlier one (groups run in start date order).
//   - Legacy number: the hub has no warranty number; its public page and QR
//     codes are /warranty/{service_no}. The legacy service number becomes
//     the public_code of the service's first warranty (the others get
//     random codes). A number whose warranties were all merged into another
//     service's warranty stays resolvable as an alias
//     (warranty_public_code_aliases) of the warranty that was kept.
//   - Period: the legacy start / end dates are calendar days in the
//     organization's time zone; end_at is the last instant of the end day
//     (decision 4 of TEC-98, like the warranty use case).
//   - Status: a warranty past its end is written expired (the expiry cron
//     has nothing to do); one the hub deactivated before its end is void;
//     an active one whose 30 / 7 day reminder window has already started is
//     stamped as notified, so the reminder cron sends nothing for history.
//   - Transfers: the hub's completed service_customer_transfers become
//     completed vehicle_transfers of the service's vehicle (from / to the
//     users the legacy customers map to, the new customer's phone as
//     to_phone). Codes are not carried over. The last transfer of a service
//     moves the vehicle (when it is still the previous owner's) and the
//     service's active warranties to the new owner. Other statuses are
//     reported, not imported.
//
// Legacy warranties and transfers are always read in full (8.8k rows), as a
// duplicate group needs all its rows; reruns are idempotent through
// migration_map. Nothing is written to the outbox and the stock ledger is
// not touched.
type WarrantiesStep struct {
	// System is the migration_map source system; empty means SourceHub.
	System string
}

// Name implements Step.
func (WarrantiesStep) Name() string { return "warranties" }

func (s WarrantiesStep) system() string {
	if s.System == "" {
		return SourceHub
	}
	return s.System
}

// Warranty statuses (chk_warranties_status).
const (
	warrantyActive  = "active"
	warrantyExpired = "expired"
	warrantyVoid    = "void"
)

// LegacyWarrantyVoidReason is the void_reason of a warranty the hub
// deactivated before its end.
const LegacyWarrantyVoidReason = "legacy: deactivated in the old hub"

// legacyTransferCodeHash is the code hash of an imported transfer: not a
// hash any code produces, and an imported transfer is never verified again.
const legacyTransferCodeHash = "!legacy-import"

// Alias reasons (warranty_public_code_aliases.reason).
const (
	aliasReasonMerged = "merged"
	aliasReasonLegacy = "legacy_number"
)

// legacyTransferMinTTL keeps expires_at after created_at (the application's
// code lifetime).
const legacyTransferMinTTL = 15 * time.Minute

// The item that a warranty covers is the lowest legacy service item with the
// same service and stock item (the hub keys warranties by those two).
const legacyWarrantiesQuery = `SELECT w.id, w.service_id, w.stock_item_id, w.start_date, w.end_date, w.is_active,
	w.created_at, w.updated_at, COALESCE(s.service_no, ''),
	(SELECT MIN(si.id) FROM service_items si WHERE si.service_id = w.service_id AND si.stock_item_id = w.stock_item_id)
FROM warranties w
LEFT JOIN services s ON s.id = w.service_id
ORDER BY w.id`

const legacyTransfersQuery = `SELECT id, service_id, current_customer_id, new_customer_id, transferred_at, transferred_by,
	status, created_at, updated_at
FROM service_customer_transfers
ORDER BY id`

type legacyWarranty struct {
	ID, ServiceID, StockItemID int64
	Start, End                 sql.NullTime
	Active                     bool
	CreatedAt, UpdatedAt       sql.NullTime
	ServiceNo                  string
	ItemID                     sql.NullInt64
}

type legacyTransfer struct {
	ID, ServiceID                int64
	CurrentCustomer, NewCustomer int64
	TransferredAt                sql.NullTime
	TransferredBy                sql.NullInt64
	Status                       string
	CreatedAt, UpdatedAt         sql.NullTime
}

// warrantyGroup is the legacy warranties of one service item.
type warrantyGroup struct {
	Kept legacyWarranty
	// All is the whole group in id order, Kept included.
	All []legacyWarranty
	// Start is the kept start date, End the latest end date.
	Start, End time.Time
	Active     bool
}

// civilDate is the calendar day of a legacy DATE (drivers return it at
// midnight, UTC or the connection zone).
func civilDate(t time.Time) (int, time.Month, int) { return t.Date() }

// dayNumber orders legacy dates by calendar day.
func dayNumber(t time.Time) int {
	y, m, d := civilDate(t)
	return y*10000 + int(m)*100 + d
}

// groupLegacyWarranties applies the duplicate rule: one group per (service,
// stock item); the kept row has the earliest start date, then the lowest id.
// Rows without both dates are returned apart (reported). Groups come in
// (start, kept id) order, so an earlier warranty is written first.
func groupLegacyWarranties(list []legacyWarranty) (groups []warrantyGroup, undated []legacyWarranty) {
	type key struct{ service, stock int64 }
	byKey := map[key]*warrantyGroup{}
	var order []key
	for _, lw := range list {
		if !lw.Start.Valid || !lw.End.Valid {
			undated = append(undated, lw)
			continue
		}
		k := key{lw.ServiceID, lw.StockItemID}
		g, ok := byKey[k]
		if !ok {
			g = &warrantyGroup{}
			byKey[k] = g
			order = append(order, k)
		}
		g.All = append(g.All, lw)
	}
	for _, k := range order {
		g := byKey[k]
		slices.SortFunc(g.All, func(a, b legacyWarranty) int { return cmp.Compare(a.ID, b.ID) })
		g.Kept = g.All[0]
		for _, lw := range g.All[1:] {
			if dayNumber(lw.Start.Time) < dayNumber(g.Kept.Start.Time) {
				g.Kept = lw
			}
		}
		g.Start, g.End = g.Kept.Start.Time, g.Kept.End.Time
		for _, lw := range g.All {
			if dayNumber(lw.End.Time) > dayNumber(g.End) {
				g.End = lw.End.Time
			}
			g.Active = g.Active || lw.Active
		}
		groups = append(groups, *g)
	}
	slices.SortStableFunc(groups, func(a, b warrantyGroup) int {
		if c := cmp.Compare(dayNumber(a.Start), dayNumber(b.Start)); c != 0 {
			return c
		}
		return cmp.Compare(a.Kept.ID, b.Kept.ID)
	})
	return groups, undated
}

// legacyWarrantyPeriod turns the legacy start / end days into instants in
// loc: the start of the first day and the last instant of the end day.
func legacyWarrantyPeriod(start, end time.Time, loc *time.Location) (time.Time, time.Time) {
	sy, sm, sd := civilDate(start)
	ey, em, ed := civilDate(end)
	return time.Date(sy, sm, sd, 0, 0, 0, 0, loc), time.Date(ey, em, ed+1, 0, 0, 0, 0, loc).Add(-time.Microsecond)
}

// warrantyState is the status a migrated warranty is written with.
type warrantyState struct {
	Status                string
	ExpiredAt, VoidedAt   pgtype.Timestamptz
	VoidReason            pgtype.Text
	Notified30, Notified7 pgtype.Timestamptz
}

// legacyWarrantyState: past its end -> expired at its end; deactivated in
// the hub before its end -> void; else active, with the reminders whose
// window has already started stamped at now.
func legacyWarrantyState(active bool, endAt, now time.Time, updated sql.NullTime) warrantyState {
	at := func(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
	switch {
	case !endAt.After(now):
		return warrantyState{Status: warrantyExpired, ExpiredAt: at(endAt)}
	case !active:
		voided := now
		if updated.Valid {
			voided = updated.Time
		}
		return warrantyState{Status: warrantyVoid, VoidedAt: at(voided),
			VoidReason: pgtype.Text{String: LegacyWarrantyVoidReason, Valid: true}}
	}
	st := warrantyState{Status: warrantyActive}
	if !endAt.After(now.AddDate(0, 0, 30)) {
		st.Notified30 = at(now)
	}
	if !endAt.After(now.AddDate(0, 0, 7)) {
		st.Notified7 = at(now)
	}
	return st
}

func locationOf(zone string) *time.Location {
	if zone == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return time.UTC
	}
	return loc
}

type warrantyRun struct {
	step WarrantiesStep
	q    *db.Queries
	m    *Mapper
	c    counts
	now  time.Time
	// svc resolves customers and users like the services step.
	svc *serviceRun
}

func (r *warrantyRun) report(key string, id int64) {
	r.c.inc(key)
	r.c.inc(key + ":" + strconv.FormatInt(id, 10))
}

// skipGroup reports a skipped group: the total counts groups, and every row
// of the group gets its own id key, as the validation report (report.go)
// matches skipped rows by id.
func (r *warrantyRun) skipGroup(key string, g warrantyGroup) {
	r.report(key, g.Kept.ID)
	for _, lw := range g.All {
		if lw.ID != g.Kept.ID {
			r.c.inc(key + ":" + strconv.FormatInt(lw.ID, 10))
		}
	}
}

func (r *warrantyRun) key(id int64) Key {
	return Key{System: r.step.system(), Table: "warranties", ID: strconv.FormatInt(id, 10), TargetTable: "warranties"}
}

// Run implements Step.
func (s WarrantiesStep) Run(ctx context.Context, src Sources, dst *Target, m *Mapper) (StepResult, error) {
	c := counts{}
	hub, err := src.Get(SourceHub)
	if err != nil {
		return StepResult{}, err
	}
	list, err := readLegacyWarranties(ctx, hub)
	if err != nil {
		return StepResult{Counts: c}, err
	}
	transfers, err := readLegacyTransfers(ctx, hub)
	if err != nil {
		return StepResult{Counts: c}, err
	}
	var watermark time.Time
	mark := func(ts ...sql.NullTime) {
		if t := latest(ts...); t.After(watermark) {
			watermark = t
		}
	}
	for _, lw := range list {
		mark(lw.CreatedAt, lw.UpdatedAt)
	}
	for _, t := range transfers {
		mark(t.CreatedAt, t.UpdatedAt, t.TransferredAt)
	}

	r := &warrantyRun{step: s, q: dst.Q, m: m, c: c, now: time.Now(),
		svc: &serviceRun{step: ServicesStep(s), dst: dst, q: dst.Q, m: m, c: c,
			orgs: map[int64]*db.MigratorOrganizationByUUIDRow{}, users: map[int64]int64{}}}

	groups, undated := groupLegacyWarranties(list)
	for _, lw := range undated {
		c.inc("warranties_read")
		r.report("warranty_skipped_period_invalid", lw.ID)
	}
	for _, g := range groups {
		c.add("warranties_read", int64(len(g.All)))
		if err := r.importGroup(ctx, g); err != nil {
			return StepResult{Counts: c}, fmt.Errorf("warranty %d: %w", g.Kept.ID, err)
		}
	}
	if err := r.aliases(ctx, list); err != nil {
		return StepResult{Counts: c}, err
	}

	// The last completed transfer of a service decides its current owner.
	last := map[int64]legacyTransfer{}
	for _, t := range transfers {
		if !legacyTransferCompleted(t) {
			continue
		}
		if p, ok := last[t.ServiceID]; !ok || transferAfter(t, p) {
			last[t.ServiceID] = t
		}
	}
	for _, t := range transfers {
		if err := r.importTransfer(ctx, t, last[t.ServiceID].ID == t.ID); err != nil {
			return StepResult{Counts: c}, fmt.Errorf("transfer %d: %w", t.ID, err)
		}
	}
	return StepResult{Counts: c, Watermark: watermark}, nil
}

func readLegacyWarranties(ctx context.Context, hub source.LegacySource) ([]legacyWarranty, error) {
	rows, err := hub.Query(ctx, legacyWarrantiesQuery)
	if err != nil {
		return nil, err
	}
	var out []legacyWarranty
	for rows.Next() {
		var lw legacyWarranty
		if err := rows.Scan(&lw.ID, &lw.ServiceID, &lw.StockItemID, &lw.Start, &lw.End, &lw.Active,
			&lw.CreatedAt, &lw.UpdatedAt, &lw.ServiceNo, &lw.ItemID); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan warranty: %w", err)
		}
		out = append(out, lw)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("read warranties: %w", err)
	}
	return out, nil
}

func readLegacyTransfers(ctx context.Context, hub source.LegacySource) ([]legacyTransfer, error) {
	rows, err := hub.Query(ctx, legacyTransfersQuery)
	if err != nil {
		return nil, err
	}
	var out []legacyTransfer
	for rows.Next() {
		var t legacyTransfer
		if err := rows.Scan(&t.ID, &t.ServiceID, &t.CurrentCustomer, &t.NewCustomer, &t.TransferredAt,
			&t.TransferredBy, &t.Status, &t.CreatedAt, &t.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan transfer: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("read transfers: %w", err)
	}
	return out, nil
}

// groupChecksum covers every row of the group, so a new duplicate or a
// changed row is a change of the group.
func groupChecksum(g warrantyGroup) string {
	vals := []any{g.Kept.ID, g.Kept.ServiceID, g.Kept.StockItemID, g.Kept.ServiceNo, g.Kept.ItemID.Int64, g.Kept.ItemID.Valid}
	for _, lw := range g.All {
		vals = append(vals, lw.ID, lw.Start.Time.Format(time.DateOnly), lw.End.Time.Format(time.DateOnly), lw.Active)
	}
	return Checksum(vals...)
}

// groupWarranty is the live warranty an earlier run mapped a row of the
// group to (the kept row first); nil when there is none.
func (r *warrantyRun) groupWarranty(ctx context.Context, g warrantyGroup) (*db.MigratorWarrantyByUUIDRow, error) {
	ids := []int64{g.Kept.ID}
	for _, lw := range g.All {
		if lw.ID != g.Kept.ID {
			ids = append(ids, lw.ID)
		}
	}
	for _, id := range ids {
		k := r.key(id)
		target, ok, err := r.m.Lookup(ctx, k.System, k.Table, k.ID)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		row, err := r.q.MigratorWarrantyByUUID(ctx, target)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read warranty: %w", err)
		}
		return &row, nil
	}
	return nil, nil
}

// mapGroup maps every row of the group to target: the kept row with the
// group checksum (changed reports whether that is a change), the others
// without one. It returns how many rows were newly mapped as merged.
func (r *warrantyRun) mapGroup(ctx context.Context, g warrantyGroup, target uuid.UUID) (changed bool, merged int64, err error) {
	sum := groupChecksum(g)
	kept := r.key(g.Kept.ID)
	cur, mapped, err := r.m.Lookup(ctx, kept.System, kept.Table, kept.ID)
	if err != nil {
		return false, 0, err
	}
	switch {
	case !mapped:
		ok, err := r.m.Link(ctx, kept, target, sum)
		if err != nil {
			return false, 0, err
		}
		changed = ok
	case cur != target:
		// The kept row maps elsewhere (a new earlier duplicate in delta
		// mode); the mapping stays, the group is reported.
		r.report("warranty_group_split", g.Kept.ID)
	default:
		res, err := r.m.Upsert(ctx, kept, sum)
		if err != nil {
			return false, 0, err
		}
		changed = res.Changed
	}
	for _, lw := range g.All {
		if lw.ID == g.Kept.ID {
			continue
		}
		ok, err := r.m.Link(ctx, r.key(lw.ID), target, "")
		if err != nil {
			return false, 0, err
		}
		if ok {
			merged++
		}
	}
	return changed, merged, nil
}

// item resolves the service item of a group (nil: skipped, reported).
func (r *warrantyRun) item(ctx context.Context, g warrantyGroup) (*db.MigratorWarrantyServiceItemRow, error) {
	if !g.Kept.ItemID.Valid {
		r.skipGroup("warranty_skipped_item_missing", g)
		return nil, nil
	}
	target, ok, err := r.m.Lookup(ctx, r.step.system(), "service_items", strconv.FormatInt(g.Kept.ItemID.Int64, 10))
	if err != nil {
		return nil, err
	}
	if !ok {
		r.skipGroup("warranty_skipped_item_unmapped", g)
		return nil, nil
	}
	row, err := r.q.MigratorWarrantyServiceItem(ctx, target)
	if errors.Is(err, pgx.ErrNoRows) {
		r.skipGroup("warranty_skipped_item_unmapped", g)
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read service item: %w", err)
	}
	return &row, nil
}

func (r *warrantyRun) importGroup(ctx context.Context, g warrantyGroup) error {
	existing, err := r.groupWarranty(ctx, g)
	if err != nil {
		return err
	}
	if existing != nil {
		changed, merged, err := r.mapGroup(ctx, g, existing.Uuid)
		if err != nil {
			return err
		}
		r.c.add("warranties_merged", merged)
		if !changed {
			r.c.inc("warranties_unchanged")
			return nil
		}
		return r.refresh(ctx, g, existing)
	}

	it, err := r.item(ctx, g)
	if err != nil || it == nil {
		return err
	}
	if it.ServiceStatus != serviceCompleted {
		r.skipGroup("warranty_skipped_service_not_completed", g)
		return nil
	}
	if it.Corrected {
		r.skipGroup("warranty_skipped_item_corrected", g)
		return nil
	}
	// A warranty the application opened for the item (repair scan).
	if w, err := r.q.MigratorWarrantyByServiceItem(ctx, it.ID); err == nil {
		_, merged, err := r.mapGroup(ctx, g, w.Uuid)
		if err != nil {
			return err
		}
		r.c.add("warranties_merged", merged)
		r.report("warranty_linked_existing", g.Kept.ID)
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("read item warranty: %w", err)
	}

	startAt, endAt := legacyWarrantyPeriod(g.Start, g.End, locationOf(it.Timezone))
	if !endAt.After(startAt) {
		r.skipGroup("warranty_skipped_period_invalid", g)
		return nil
	}
	st := legacyWarrantyState(g.Active, endAt, r.now, g.Kept.UpdatedAt)
	if st.Status == warrantyActive && it.Kind == "full" {
		other, err := r.q.MigratorActiveFullWarranty(ctx, db.MigratorActiveFullWarrantyParams{VehicleID: it.VehicleID, UnitID: it.UnitID})
		if err == nil {
			// The same unit on the same vehicle under another service: a
			// duplicate of the earlier warranty.
			_, merged, err := r.mapGroup(ctx, g, other.Uuid)
			if err != nil {
				return err
			}
			r.c.add("warranties_merged", merged+1)
			r.report("warranty_merged_unit", g.Kept.ID)
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read unit warranty: %w", err)
		}
	}
	code, err := r.publicCode(ctx, g, it.ServiceID)
	if err != nil {
		return err
	}

	res, err := r.m.Upsert(ctx, r.key(g.Kept.ID), groupChecksum(g))
	if err != nil {
		return err
	}
	if _, err := r.q.MigratorInsertWarranty(ctx, db.MigratorInsertWarrantyParams{
		Uuid: res.UUID, PublicCode: code, OrganizationID: it.OrganizationID, BrandID: it.BrandID,
		ServiceID: it.ServiceID, ServiceItemID: it.ID, ProductID: it.ProductID, UnitID: it.UnitID,
		ItemKind: it.Kind, VehicleID: it.VehicleID, HolderUserID: it.CustomerUserID,
		StartAt: pgtype.Timestamptz{Time: startAt, Valid: true}, EndAt: pgtype.Timestamptz{Time: endAt, Valid: true},
		Status: st.Status, ExpiredAt: st.ExpiredAt, VoidedAt: st.VoidedAt, VoidReason: st.VoidReason,
		Notified30At: st.Notified30, Notified7At: st.Notified7,
		CreatedAt: pgTime(g.Kept.CreatedAt), UpdatedAt: pgTime(g.Kept.UpdatedAt),
	}); err != nil {
		return fmt.Errorf("insert warranty: %w", err)
	}
	_, merged, err := r.mapGroup(ctx, g, res.UUID)
	if err != nil {
		return err
	}
	r.c.add("warranties_merged", merged)
	r.c.inc("warranties_created")
	r.c.inc("warranties_" + st.Status)
	if code.Valid {
		r.c.inc("public_codes_kept")
	}
	return nil
}

// publicCode is the legacy service number when it can be the warranty's
// public code: well formed and not already a code. NULL takes a random one.
func (r *warrantyRun) publicCode(ctx context.Context, g warrantyGroup, serviceID int64) (pgtype.Text, error) {
	no := strings.TrimSpace(g.Kept.ServiceNo)
	if !warranty.ValidPublicCode(no) {
		r.report("public_code_invalid", g.Kept.ID)
		return pgtype.Text{}, nil
	}
	owner, err := r.q.MigratorPublicCodeOwner(ctx, no)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return pgtype.Text{String: no, Valid: true}, nil
	case err != nil:
		return pgtype.Text{}, fmt.Errorf("public code: %w", err)
	case owner != serviceID:
		// Another service's warranty holds the number.
		r.report("public_code_taken", g.Kept.ID)
	}
	return pgtype.Text{}, nil
}

// refresh applies a changed legacy group to the warranty it became. Only an
// active warranty of the group's own service changes (end, status).
func (r *warrantyRun) refresh(ctx context.Context, g warrantyGroup, w *db.MigratorWarrantyByUUIDRow) error {
	it, err := r.item(ctx, g)
	if err != nil {
		return err
	}
	if it == nil || it.ServiceID != w.ServiceID || w.Status != warrantyActive {
		r.report("warranty_changed_kept", g.Kept.ID)
		return nil
	}
	_, endAt := legacyWarrantyPeriod(g.Start, g.End, locationOf(it.Timezone))
	st := legacyWarrantyState(g.Active, endAt, r.now, g.Kept.UpdatedAt)
	if st.Status == warrantyActive && w.EndAt.Time.Equal(endAt) {
		r.c.inc("warranties_unchanged")
		return nil
	}
	n, err := r.q.MigratorUpdateActiveWarranty(ctx, db.MigratorUpdateActiveWarrantyParams{
		ID: w.ID, EndAt: pgtype.Timestamptz{Time: endAt, Valid: true}, Status: st.Status,
		ExpiredAt: st.ExpiredAt, VoidedAt: st.VoidedAt, VoidReason: st.VoidReason,
		Notified30At: st.Notified30, Notified7At: st.Notified7,
	})
	if err != nil {
		return fmt.Errorf("update warranty: %w", err)
	}
	if n == 0 {
		r.report("warranty_changed_kept", g.Kept.ID)
		return nil
	}
	r.c.inc("warranties_updated")
	return nil
}

// aliases keeps every legacy service number with migrated warranties
// resolvable: a number that is neither a public code nor an alias yet
// becomes an alias of a warranty its warranties were mapped to.
func (r *warrantyRun) aliases(ctx context.Context, list []legacyWarranty) error {
	type legacySvc struct {
		no      string
		ids     []int64
		created sql.NullTime
	}
	byService := map[int64]*legacySvc{}
	var order []int64
	for _, lw := range list {
		s, ok := byService[lw.ServiceID]
		if !ok {
			s = &legacySvc{no: strings.TrimSpace(lw.ServiceNo)}
			byService[lw.ServiceID] = s
			order = append(order, lw.ServiceID)
		}
		s.ids = append(s.ids, lw.ID)
		if lw.CreatedAt.Valid && (!s.created.Valid || lw.CreatedAt.Time.Before(s.created.Time)) {
			s.created = lw.CreatedAt
		}
	}
	slices.Sort(order)
	for _, sid := range order {
		s := byService[sid]
		if !warranty.ValidPublicCode(s.no) {
			continue
		}
		if _, err := r.q.MigratorWarrantyAliasByCode(ctx, s.no); err == nil {
			r.c.inc("aliases_unchanged")
			continue
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read alias: %w", err)
		}
		if _, err := r.q.MigratorPublicCodeOwner(ctx, s.no); err == nil {
			continue // the number is a public code
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("public code: %w", err)
		}
		var target *db.MigratorWarrantyByUUIDRow
		for _, id := range s.ids {
			k := r.key(id)
			u, ok, err := r.m.Lookup(ctx, k.System, k.Table, k.ID)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			row, err := r.q.MigratorWarrantyByUUID(ctx, u)
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return fmt.Errorf("read warranty: %w", err)
			}
			target = &row
			break
		}
		if target == nil {
			// None of the service's warranties was migrated (reported).
			continue
		}
		reason := aliasReasonMerged
		if svcUUID, ok, err := r.m.Lookup(ctx, r.step.system(), "services", strconv.FormatInt(sid, 10)); err != nil {
			return err
		} else if ok {
			svc, err := r.q.MigratorServiceByUUID(ctx, svcUUID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("read service: %w", err)
			}
			if err == nil && svc.ID == target.ServiceID {
				reason = aliasReasonLegacy
			}
		}
		key := Key{System: r.step.system(), Table: "services.warranty_number", ID: strconv.FormatInt(sid, 10),
			TargetTable: "warranty_public_code_aliases"}
		res, err := r.m.Upsert(ctx, key, Checksum(s.no, target.ID))
		if err != nil {
			return err
		}
		if _, err := r.q.MigratorInsertWarrantyAlias(ctx, db.MigratorInsertWarrantyAliasParams{
			Uuid: res.UUID, Code: s.no, WarrantyID: target.ID, Reason: reason, CreatedAt: pgTime(s.created),
		}); err != nil {
			return fmt.Errorf("insert alias: %w", err)
		}
		r.c.inc("aliases_created")
	}
	return nil
}

func legacyTransferCompleted(t legacyTransfer) bool {
	return strings.ToLower(strings.TrimSpace(t.Status)) == "completed"
}

// transferTime is when a legacy transfer was completed (best known time).
func transferTime(t legacyTransfer) time.Time {
	for _, ts := range []sql.NullTime{t.TransferredAt, t.UpdatedAt, t.CreatedAt} {
		if ts.Valid {
			return ts.Time
		}
	}
	return time.Time{}
}

// transferAfter orders transfers by completion time, then id.
func transferAfter(a, b legacyTransfer) bool {
	if c := transferTime(a).Compare(transferTime(b)); c != 0 {
		return c > 0
	}
	return a.ID > b.ID
}

// legacyTransferTimes are the created / completed / expires instants of an
// imported transfer (expires_at must be after created_at).
func legacyTransferTimes(t legacyTransfer, now time.Time) (created, completed, expires time.Time) {
	completed = transferTime(t)
	if completed.IsZero() {
		completed = now
	}
	created = completed
	if t.CreatedAt.Valid && !t.CreatedAt.Time.After(completed) {
		created = t.CreatedAt.Time
	}
	expires = created.Add(legacyTransferMinTTL)
	if completed.After(expires) {
		expires = completed
	}
	return created, completed, expires
}

func (r *warrantyRun) importTransfer(ctx context.Context, t legacyTransfer, last bool) error {
	r.c.inc("transfers_read")
	key := Key{System: r.step.system(), Table: "service_customer_transfers", ID: strconv.FormatInt(t.ID, 10),
		TargetTable: "vehicle_transfers"}
	sum := Checksum(t.ServiceID, t.CurrentCustomer, t.NewCustomer, t.TransferredAt.Time, t.TransferredAt.Valid,
		t.TransferredBy.Int64, t.TransferredBy.Valid, t.Status)
	target, mapped, err := r.m.Lookup(ctx, key.System, key.Table, key.ID)
	if err != nil {
		return err
	}
	if mapped {
		_, err := r.q.MigratorVehicleTransferByUUID(ctx, target)
		switch {
		case err == nil:
			res, err := r.m.Upsert(ctx, key, sum)
			if err != nil {
				return err
			}
			if res.Changed {
				// A completed transfer is final here.
				r.report("transfer_changed_kept", t.ID)
			} else {
				r.c.inc("transfers_unchanged")
			}
			return nil
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("read transfer: %w", err)
		}
	}
	if !legacyTransferCompleted(t) {
		r.c.inc("transfer_status:" + truncate(strings.ToLower(strings.TrimSpace(t.Status)), 32))
		r.report("transfer_skipped_not_completed", t.ID)
		return nil
	}
	svcUUID, ok, err := r.m.Lookup(ctx, r.step.system(), "services", strconv.FormatInt(t.ServiceID, 10))
	if err != nil {
		return err
	}
	if !ok {
		r.report("transfer_skipped_service_unmapped", t.ID)
		return nil
	}
	svc, err := r.q.MigratorTransferService(ctx, svcUUID)
	if errors.Is(err, pgx.ErrNoRows) {
		r.report("transfer_skipped_service_unmapped", t.ID)
		return nil
	}
	if err != nil {
		return fmt.Errorf("read service: %w", err)
	}
	from, err := r.svc.customerUser(ctx, t.CurrentCustomer)
	if err != nil {
		return err
	}
	to, err := r.svc.customerUser(ctx, t.NewCustomer)
	if err != nil {
		return err
	}
	switch {
	case from == 0 || to == 0:
		r.report("transfer_skipped_customer_unmapped", t.ID)
		return nil
	case from == to:
		// Both legacy customers are one person here (merged accounts).
		r.report("transfer_skipped_same_user", t.ID)
		return nil
	}
	phone, err := r.q.MigratorUserPhone(ctx, to)
	if err != nil {
		return fmt.Errorf("read phone: %w", err)
	}
	if !phone.Valid {
		r.report("transfer_skipped_no_phone", t.ID)
		return nil
	}
	initiator := pgtype.Int8{}
	if t.TransferredBy.Valid {
		id, err := r.svc.user(ctx, t.TransferredBy.Int64)
		if err != nil {
			return err
		}
		if id != 0 {
			initiator = pgInt8(id)
		} else {
			r.report("transfer_user_unmapped", t.ID)
		}
	}
	created, completed, expires := legacyTransferTimes(t, r.now)
	res, err := r.m.Upsert(ctx, key, sum)
	if err != nil {
		return err
	}
	ts := func(v time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: v, Valid: true} }
	if _, err := r.q.MigratorInsertCompletedVehicleTransfer(ctx, db.MigratorInsertCompletedVehicleTransferParams{
		Uuid: res.UUID, OrganizationID: svc.OrganizationID, BrandID: svc.BrandID, VehicleID: svc.VehicleID,
		FromUserID: from, ToUserID: to, ToPhone: phone.String, CodeHash: legacyTransferCodeHash,
		CompletedAt: ts(completed), ExpiresAt: ts(expires), InitiatedByUserID: initiator, CreatedAt: ts(created),
	}); err != nil {
		return fmt.Errorf("insert transfer: %w", err)
	}
	r.c.inc("transfers_created")
	if !last {
		return nil
	}
	// The service's last transfer: the vehicle and the service's active
	// warranties belong to the new owner.
	switch svc.VehicleUserID {
	case to:
	case from:
		n, err := r.q.MigratorMoveVehicleOwner(ctx, db.MigratorMoveVehicleOwnerParams{ID: svc.VehicleID, OldUserID: from, NewUserID: to})
		if err != nil {
			return fmt.Errorf("move vehicle: %w", err)
		}
		r.c.add("vehicles_moved", n)
	default:
		r.report("transfer_vehicle_owner_mismatch", t.ID)
	}
	n, err := r.q.MigratorSetServiceWarrantyHolder(ctx, db.MigratorSetServiceWarrantyHolderParams{ServiceID: svc.ID, HolderUserID: to})
	if err != nil {
		return fmt.Errorf("move warranty holders: %w", err)
	}
	r.c.add("warranty_holders_moved", n)
	return nil
}

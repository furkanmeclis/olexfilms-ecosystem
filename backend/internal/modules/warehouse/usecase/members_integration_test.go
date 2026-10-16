package usecase_test

import (
	"errors"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	wh "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-769 acceptance against a migrated PostgreSQL (TEST_DATABASE_URL): a
// user without memberships writes in every warehouse of the organization;
// once a member of one warehouse, stock entries, transfers, counts and bin
// moves elsewhere are refused with ErrNotWarehouseMember; removing the last
// membership lifts the restriction. The member default feeds the mobile
// stock entry without warehouse_uuid.

func (e *entryEnv) orgUser(t *testing.T, o db.Organization, tag string) db.User {
	t.Helper()
	u, err := e.q.CreateUser(e.ctx, db.CreateUserParams{
		PasswordHash: "x", Name: "T769 " + tag, Surname: e.suffix, Status: "active",
		Email: pgtype.Text{String: "t769-" + tag + "-" + e.suffix + "@example.test", Valid: true},
	})
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	if _, err := e.q.CreateOrganizationMember(e.ctx, db.CreateOrganizationMemberParams{
		OrganizationID: o.ID, UserID: u.ID, Role: "staff",
	}); err != nil {
		t.Fatalf("org member: %v", err)
	}
	return u
}

func (e *entryEnv) userCaller(o db.Organization, u db.User) wh.EntryCaller {
	c := e.caller(o)
	c.Principal = authctx.Principal{UserID: u.Uuid, UserInternal: u.ID}
	return c
}

func TestWarehouseMembershipLimitsWrites(t *testing.T) {
	e := newEntryEnv(t)
	members := wh.NewMembers(e.pool, e.q)
	transfers := wh.NewWarehouseTransfers(e.pool, e.q, outbox.NewStore(e.pool, e.q))
	counts := wh.NewCounts(e.pool, e.q, outbox.NewStore(e.pool, e.q), nil)
	admin := e.caller(e.dist)
	w1, loc1 := e.location(t, e.dist, "MA")
	w2, loc2 := e.location(t, e.dist, "MB")
	user := e.orgUser(t, e.dist, "staff")
	uc := e.userCaller(e.dist, user)
	scanCaller := wh.ScanCaller{Caller: uc.Caller, Principal: uc.Principal}

	// No membership: every warehouse is writable.
	mine, err := members.Mine(e.ctx, uc)
	if err != nil {
		t.Fatalf("mine: %v", err)
	}
	if mine.Restricted || len(mine.Items) < 2 || mine.DefaultUUID != nil {
		t.Fatalf("unrestricted mine = %+v", mine)
	}
	for _, it := range mine.Items {
		if !it.Member {
			t.Fatalf("unrestricted item not writable: %+v", it)
		}
	}
	if _, err := e.entries.Create(e.ctx, uc, wh.EntryInput{WarehouseUUID: w2.UUID.String(), Mode: wh.EntryModeWithExisting}); err != nil {
		t.Fatalf("free entry in w2: %v", err)
	}
	if _, err := transfers.CreateTransfer(e.ctx, uc, wh.TransferInput{FromWarehouseUUID: w2.UUID.String(), ToWarehouseUUID: w1.UUID.String()}); err != nil {
		t.Fatalf("free transfer w2->w1: %v", err)
	}

	// A draft of w2 opened by an unrestricted admin, touched later.
	w2Entry, err := e.entries.Create(e.ctx, admin, wh.EntryInput{WarehouseUUID: w2.UUID.String(), Mode: wh.EntryModeWithExisting})
	if err != nil {
		t.Fatalf("admin entry: %v", err)
	}
	w2Transfer, err := transfers.CreateTransfer(e.ctx, admin, wh.TransferInput{FromWarehouseUUID: w2.UUID.String(), ToWarehouseUUID: w1.UUID.String()})
	if err != nil {
		t.Fatalf("admin transfer: %v", err)
	}

	// Membership of w1 (default) turns the restriction on.
	m, err := members.Add(e.ctx, admin, w1.UUID.String(), wh.MemberInput{UserUUID: user.Uuid.String(), IsDefault: true})
	if err != nil {
		t.Fatalf("add member: %v", err)
	}
	if !m.IsDefault || m.User.UUID != user.Uuid {
		t.Fatalf("member = %+v", m)
	}
	if _, err := members.Add(e.ctx, admin, w1.UUID.String(), wh.MemberInput{UserUUID: user.Uuid.String()}); err != nil {
		t.Fatalf("add member again (idempotent): %v", err)
	}
	stranger := e.orgUser(t, e.center, "stranger")
	if _, err := members.Add(e.ctx, admin, w1.UUID.String(), wh.MemberInput{UserUUID: stranger.Uuid.String()}); !errors.Is(err, wh.ErrMemberNotFound) {
		t.Fatalf("add non org member = %v, want ErrMemberNotFound", err)
	}
	list, total, err := members.List(e.ctx, admin.Caller, w1.UUID.String(), wh.MemberListFilter{Q: "staff", Limit: 10})
	if err != nil || total != 1 || len(list) != 1 || list[0].User.UUID != user.Uuid {
		t.Fatalf("list = %+v total %d err %v", list, total, err)
	}

	mine, err = members.Mine(e.ctx, uc)
	if err != nil {
		t.Fatalf("mine: %v", err)
	}
	if !mine.Restricted || mine.DefaultUUID == nil || *mine.DefaultUUID != w1.UUID {
		t.Fatalf("restricted mine = %+v", mine)
	}
	for _, it := range mine.Items {
		if it.UUID == w2.UUID && it.Member {
			t.Fatalf("w2 still writable: %+v", it)
		}
	}

	notMember := func(name string, err error) {
		t.Helper()
		if !errors.Is(err, wh.ErrNotWarehouseMember) {
			t.Fatalf("%s = %v, want ErrNotWarehouseMember", name, err)
		}
	}
	_, err = e.entries.Create(e.ctx, uc, wh.EntryInput{WarehouseUUID: w2.UUID.String(), Mode: wh.EntryModeWithExisting})
	notMember("entry create w2", err)
	_, err = e.entries.Place(e.ctx, uc, w2Entry.UUID, wh.PlaceInput{LocationUUID: &loc2.UUID})
	notMember("entry place w2", err)
	_, err = e.entries.Cancel(e.ctx, uc, w2Entry.UUID)
	notMember("entry cancel w2", err)
	notMember("entry import check w2", e.entries.CheckWrite(e.ctx, uc, w2Entry.UUID))
	_, err = transfers.CreateTransfer(e.ctx, uc, wh.TransferInput{FromWarehouseUUID: w2.UUID.String(), ToWarehouseUUID: w1.UUID.String()})
	notMember("transfer create from w2", err)
	_, err = transfers.AddTransferLines(e.ctx, uc, w2Transfer.UUID, []string{"T769-NONE"})
	notMember("transfer lines from w2", err)
	_, err = transfers.Ship(e.ctx, uc, w2Transfer.UUID)
	notMember("transfer ship from w2", err)
	_, err = transfers.Move(e.ctx, uc, wh.MoveInput{Barcodes: []string{"T769-NONE"}, LocationUUID: &loc2.UUID})
	notMember("move into w2", err)
	_, err = counts.Create(e.ctx, scanCaller, wh.CountInput{WarehouseUUID: w2.UUID.String(), Method: wh.CountMethodLocationFirst, Visibility: wh.CountVisibilityBlind})
	notMember("count create w2", err)

	// Member warehouse: the writes go through.
	if _, err := e.entries.Create(e.ctx, uc, wh.EntryInput{WarehouseUUID: w1.UUID.String(), Mode: wh.EntryModeWithExisting}); err != nil {
		t.Fatalf("entry in w1: %v", err)
	}
	// The mobile create without warehouse_uuid takes the member default.
	def, err := e.entries.Create(e.ctx, uc, wh.EntryInput{Mode: wh.EntryModeWithExisting, DefaultWarehouse: true})
	if err != nil || def.Warehouse == nil || def.Warehouse.UUID != w1.UUID {
		t.Fatalf("default entry = %+v, %v", def.Warehouse, err)
	}
	if _, err := transfers.CreateTransfer(e.ctx, uc, wh.TransferInput{FromWarehouseUUID: w1.UUID.String(), ToWarehouseUUID: w2.UUID.String()}); err != nil {
		t.Fatalf("transfer from w1: %v", err)
	}
	// A transfer into w1 may be cancelled by the target side.
	if _, err := transfers.Cancel(e.ctx, uc, w2Transfer.UUID); err != nil {
		t.Fatalf("cancel transfer into w1: %v", err)
	}
	if _, err := counts.Create(e.ctx, scanCaller, wh.CountInput{WarehouseUUID: w1.UUID.String(), Method: wh.CountMethodLocationFirst, Visibility: wh.CountVisibilityBlind}); err != nil {
		t.Fatalf("count in w1: %v", err)
	}
	_, err = transfers.Move(e.ctx, uc, wh.MoveInput{Barcodes: []string{"T769-NONE"}, LocationUUID: &loc1.UUID})
	if errors.Is(err, wh.ErrNotWarehouseMember) || err == nil {
		t.Fatalf("move into w1 = %v, want a unit error", err)
	}

	// The default only moves to a member warehouse.
	if _, err := members.SetDefault(e.ctx, uc, w2.UUID.String()); !errors.Is(err, wh.ErrNotWarehouseMember) {
		t.Fatalf("default w2 = %v", err)
	}

	// Removing the last membership lifts the restriction.
	if err := members.Remove(e.ctx, admin, w1.UUID.String(), user.Uuid.String()); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := members.Remove(e.ctx, admin, w1.UUID.String(), user.Uuid.String()); !errors.Is(err, wh.ErrMemberNotFound) {
		t.Fatalf("remove twice = %v", err)
	}
	if _, err := e.entries.Create(e.ctx, uc, wh.EntryInput{WarehouseUUID: w2.UUID.String(), Mode: wh.EntryModeWithExisting}); err != nil {
		t.Fatalf("entry in w2 after remove: %v", err)
	}
	if _, err := e.entries.Create(e.ctx, uc, wh.EntryInput{Mode: wh.EntryModeWithExisting, DefaultWarehouse: true}); err == nil {
		t.Fatal("default entry without a default accepted")
	}
}

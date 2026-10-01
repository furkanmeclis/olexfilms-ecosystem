package ledger

import (
	"errors"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
)

const (
	center = int64(1)
	dist   = int64(2)
	dealer = int64(3)
)

var (
	centerLoc = Owner{Type: OwnerWarehouseLocation, ID: 10, OrgID: center}
	distOrg   = Owner{Type: OwnerOrganization, ID: dist, OrgID: dist}
	distLoc   = Owner{Type: OwnerWarehouseLocation, ID: 20, OrgID: dist}
	dealerOrg = Owner{Type: OwnerOrganization, ID: dealer, OrgID: dealer}
	service   = Owner{Type: OwnerService, ID: 900, OrgID: dealer}
)

func ptr(o Owner) *Owner { return &o }

func held(s Status, o Owner) serialCurrent {
	return serialCurrent{status: s, hasState: true, owner: o, issuerOrg: center}
}

func roll(cur serialCurrent, initial, remaining int64) serialCurrent {
	cur.isRoll, cur.initial, cur.remaining = true, initial, remaining
	return cur
}

func label(s Status) serialCurrent { return serialCurrent{status: s, issuerOrg: center} }

func mv(t MovementType, to *Owner) Movement {
	return Movement{Type: t, To: to, Source: "test", RefType: "doc", RefID: 7}
}

func out(t MovementType, from Owner, s Status) *prevMovement {
	return &prevMovement{typ: t, refType: "doc", refID: 7, fromOwner: &from, fromStatus: s}
}

func TestSerialTransitions(t *testing.T) {
	type tc struct {
		name       string
		m          Movement
		cur        serialCurrent
		prev       *prevMovement
		wantErr    error
		wantStatus Status
		wantOwner  Owner
		wantQty    int32
		wantCm     int64
	}
	cases := []tc{
		// entry
		{name: "entry printed->center location", m: mv(TypeEntry, ptr(centerLoc)), cur: label(StatusPrinted),
			wantStatus: StatusAvailable, wantOwner: centerLoc, wantQty: 1},
		{name: "entry reserved->center org", m: mv(TypeEntry, &Owner{Type: OwnerOrganization, ID: center}), cur: label(StatusReserved),
			wantStatus: StatusAvailable, wantOwner: Owner{Type: OwnerOrganization, ID: center, OrgID: center}, wantQty: 1},
		{name: "entry twice", m: mv(TypeEntry, ptr(centerLoc)), cur: held(StatusAvailable, centerLoc), wantErr: ErrTransitionNotAllowed},
		{name: "entry to service", m: mv(TypeEntry, ptr(service)), cur: label(StatusPrinted), wantErr: ErrOwnerNotAllowed},
		{name: "entry without target", m: mv(TypeEntry, nil), cur: label(StatusPrinted), wantErr: ErrInvalidMovement},
		{name: "entry location without org", m: mv(TypeEntry, &Owner{Type: OwnerWarehouseLocation, ID: 10}), cur: label(StatusPrinted), wantErr: ErrInvalidMovement},
		// placement
		{name: "placement available->placed", m: mv(TypePlacement, ptr(centerLoc)), cur: held(StatusAvailable, Owner{Type: OwnerOrganization, ID: center, OrgID: center}),
			wantStatus: StatusPlaced, wantOwner: centerLoc},
		{name: "placement other org", m: mv(TypePlacement, ptr(distLoc)), cur: held(StatusAvailable, centerLoc), wantErr: ErrOwnerNotAllowed},
		{name: "placement same bin", m: mv(TypePlacement, ptr(centerLoc)), cur: held(StatusPlaced, centerLoc), wantErr: ErrInvalidMovement},
		{name: "placement in transit", m: mv(TypePlacement, ptr(centerLoc)), cur: held(StatusInTransit, distOrg), wantErr: ErrTransitionNotAllowed},
		{name: "placement to organization", m: mv(TypePlacement, ptr(distOrg)), cur: held(StatusAvailable, distOrg), wantErr: ErrOwnerNotAllowed},
		// transfer
		{name: "transfer_out placed->in_transit", m: mv(TypeTransferOut, ptr(distOrg)), cur: held(StatusPlaced, centerLoc),
			wantStatus: StatusInTransit, wantOwner: distOrg, wantQty: -1},
		{name: "transfer_out to location", m: mv(TypeTransferOut, ptr(distLoc)), cur: held(StatusPlaced, centerLoc), wantErr: ErrOwnerNotAllowed},
		{name: "transfer_out twice (second owner)", m: mv(TypeTransferOut, ptr(dealerOrg)), cur: held(StatusInTransit, distOrg), wantErr: ErrTransitionNotAllowed},
		{name: "transfer_out used", m: mv(TypeTransferOut, ptr(distOrg)), cur: held(StatusUsed, service), wantErr: ErrTransitionNotAllowed},
		{name: "transfer_in", m: mv(TypeTransferIn, ptr(distLoc)), cur: held(StatusInTransit, distOrg), prev: out(TypeTransferOut, centerLoc, StatusPlaced),
			wantStatus: StatusAvailable, wantOwner: distLoc, wantQty: 1},
		{name: "transfer_in wrong holder", m: mv(TypeTransferIn, ptr(dealerOrg)), cur: held(StatusInTransit, distOrg), prev: out(TypeTransferOut, centerLoc, StatusPlaced), wantErr: ErrOwnerNotAllowed},
		{name: "transfer_in after order_out", m: mv(TypeTransferIn, ptr(distLoc)), cur: held(StatusInTransit, distOrg), prev: out(TypeOrderOut, centerLoc, StatusPlaced), wantErr: ErrTransitionNotAllowed},
		{name: "transfer_in other reference", m: Movement{Type: TypeTransferIn, To: ptr(distLoc), RefType: "doc", RefID: 8},
			cur: held(StatusInTransit, distOrg), prev: out(TypeTransferOut, centerLoc, StatusPlaced), wantErr: ErrTransitionNotAllowed},
		{name: "transfer_in not in transit", m: mv(TypeTransferIn, ptr(distLoc)), cur: held(StatusAvailable, distOrg), wantErr: ErrTransitionNotAllowed},
		{name: "transfer_cancel_restore", m: mv(TypeTransferCancelRestore, nil), cur: held(StatusInTransit, distOrg), prev: out(TypeTransferOut, centerLoc, StatusPlaced),
			wantStatus: StatusPlaced, wantOwner: centerLoc, wantQty: 1},
		{name: "transfer_cancel_restore elsewhere", m: mv(TypeTransferCancelRestore, ptr(distLoc)), cur: held(StatusInTransit, distOrg), prev: out(TypeTransferOut, centerLoc, StatusPlaced), wantErr: ErrOwnerMismatch},
		{name: "transfer_cancel_restore without out", m: mv(TypeTransferCancelRestore, nil), cur: held(StatusInTransit, distOrg), wantErr: ErrTransitionNotAllowed},
		// order
		{name: "order_out", m: mv(TypeOrderOut, ptr(dealerOrg)), cur: held(StatusAvailable, distLoc),
			wantStatus: StatusInTransit, wantOwner: dealerOrg, wantQty: -1},
		{name: "received", m: mv(TypeReceived, ptr(dealerOrg)), cur: held(StatusInTransit, dealerOrg), prev: out(TypeOrderOut, distLoc, StatusAvailable),
			wantStatus: StatusAvailable, wantOwner: dealerOrg, wantQty: 1},
		{name: "received after transfer_out", m: mv(TypeReceived, ptr(dealerOrg)), cur: held(StatusInTransit, dealerOrg), prev: out(TypeTransferOut, distLoc, StatusAvailable), wantErr: ErrTransitionNotAllowed},
		{name: "order_cancel_restore", m: mv(TypeOrderCancelRestore, nil), cur: held(StatusInTransit, dealerOrg), prev: out(TypeOrderOut, distLoc, StatusAvailable),
			wantStatus: StatusAvailable, wantOwner: distLoc, wantQty: 1},
		{name: "order_cancel_restore not in transit", m: mv(TypeOrderCancelRestore, nil), cur: held(StatusAvailable, dealerOrg), wantErr: ErrTransitionNotAllowed},
		// consumption / return
		{name: "consumption", m: mv(TypeConsumption, ptr(service)), cur: held(StatusAvailable, dealerOrg),
			wantStatus: StatusUsed, wantOwner: service, wantQty: -1},
		{name: "double consumption", m: mv(TypeConsumption, ptr(service)), cur: held(StatusUsed, service), wantErr: ErrTransitionNotAllowed},
		{name: "consumption of another org's unit", m: mv(TypeConsumption, ptr(service)), cur: held(StatusAvailable, distOrg), wantErr: ErrOwnerNotAllowed},
		{name: "consumption to organization", m: mv(TypeConsumption, ptr(dealerOrg)), cur: held(StatusAvailable, dealerOrg), wantErr: ErrOwnerNotAllowed},
		{name: "consumption roll uses all", m: mv(TypeConsumption, ptr(service)), cur: roll(held(StatusAvailable, dealerOrg), 1500, 600),
			wantStatus: StatusUsed, wantOwner: service, wantQty: -1, wantCm: -600},
		{name: "consumption with meters", m: Movement{Type: TypeConsumption, To: ptr(service), Centimeters: 100, RefType: "doc", RefID: 7},
			cur: roll(held(StatusAvailable, dealerOrg), 1500, 600), wantErr: ErrInvalidMovement},
		{name: "return", m: mv(TypeReturn, ptr(dealerOrg)), cur: held(StatusUsed, service),
			wantStatus: StatusAvailable, wantOwner: dealerOrg, wantQty: 1},
		{name: "return to another org", m: mv(TypeReturn, ptr(distOrg)), cur: held(StatusUsed, service), wantErr: ErrOwnerNotAllowed},
		{name: "return not used", m: mv(TypeReturn, ptr(dealerOrg)), cur: held(StatusAvailable, dealerOrg), wantErr: ErrTransitionNotAllowed},
		{name: "return from trash", m: mv(TypeReturn, ptr(dealerOrg)), cur: held(StatusUsed, Owner{Type: OwnerTrash, ID: dealer, OrgID: dealer}), wantErr: ErrTransitionNotAllowed},
		{name: "return roll with meters", m: Movement{Type: TypeReturn, To: ptr(dealerOrg), Centimeters: 200, RefType: "doc", RefID: 7},
			cur: roll(held(StatusUsed, service), 1500, 0), wantStatus: StatusAvailable, wantOwner: dealerOrg, wantQty: 1, wantCm: 200},
		{name: "return roll without meters", m: mv(TypeReturn, ptr(dealerOrg)), cur: roll(held(StatusUsed, service), 1500, 0), wantErr: ErrInvalidMovement},
		// partial consumption: 15 - 5 - 4 = 6
		{name: "partial 5 of 15", m: Movement{Type: TypePartialConsumption, Centimeters: 500, RefType: "doc", RefID: 7},
			cur: roll(held(StatusAvailable, dealerOrg), 1500, 1500), wantStatus: StatusAvailable, wantOwner: dealerOrg, wantCm: -500},
		{name: "partial 4 of 10", m: Movement{Type: TypePartialConsumption, Centimeters: 400, RefType: "doc", RefID: 7},
			cur: roll(held(StatusPlaced, distLoc), 1500, 1000), wantStatus: StatusPlaced, wantOwner: distLoc, wantCm: -400},
		{name: "partial 7 of 6", m: Movement{Type: TypePartialConsumption, Centimeters: 700, RefType: "doc", RefID: 7},
			cur: roll(held(StatusAvailable, dealerOrg), 1500, 600), wantErr: ErrInsufficientMeters},
		{name: "partial rest of roll", m: Movement{Type: TypePartialConsumption, Centimeters: 600, RefType: "doc", RefID: 7},
			cur: roll(held(StatusAvailable, dealerOrg), 1500, 600), wantErr: ErrInvalidMovement},
		{name: "partial zero", m: Movement{Type: TypePartialConsumption, RefType: "doc", RefID: 7},
			cur: roll(held(StatusAvailable, dealerOrg), 1500, 600), wantErr: ErrInvalidMovement},
		{name: "partial on a piece", m: Movement{Type: TypePartialConsumption, Centimeters: 100, RefType: "doc", RefID: 7},
			cur: held(StatusAvailable, dealerOrg), wantErr: ErrTransitionNotAllowed},
		{name: "partial moving the roll", m: Movement{Type: TypePartialConsumption, To: ptr(service), Centimeters: 100, RefType: "doc", RefID: 7},
			cur: roll(held(StatusAvailable, dealerOrg), 1500, 600), wantErr: ErrInvalidMovement},
		{name: "partial used roll", m: Movement{Type: TypePartialConsumption, Centimeters: 100, RefType: "doc", RefID: 7},
			cur: roll(held(StatusUsed, service), 1500, 600), wantErr: ErrTransitionNotAllowed},
		// count adjustment
		{name: "count roll -1m", m: Movement{Type: TypeCountAdjustment, Centimeters: -100, RefType: "doc", RefID: 7},
			cur: roll(held(StatusPlaced, distLoc), 1500, 600), wantStatus: StatusPlaced, wantOwner: distLoc, wantCm: -100},
		{name: "count roll below zero", m: Movement{Type: TypeCountAdjustment, Centimeters: -700, RefType: "doc", RefID: 7},
			cur: roll(held(StatusPlaced, distLoc), 1500, 600), wantErr: ErrInsufficientMeters},
		{name: "count roll above initial", m: Movement{Type: TypeCountAdjustment, Centimeters: 1000, RefType: "doc", RefID: 7},
			cur: roll(held(StatusPlaced, distLoc), 1500, 600), wantErr: ErrInvalidMovement},
		{name: "count piece", m: Movement{Type: TypeCountAdjustment, Centimeters: 100, RefType: "doc", RefID: 7},
			cur: held(StatusPlaced, distLoc), wantErr: ErrTransitionNotAllowed},
		// void / external
		{name: "void label", m: mv(TypeVoid, nil), cur: label(StatusPrinted),
			wantStatus: StatusVoid, wantOwner: Owner{Type: OwnerTrash, ID: center, OrgID: center}},
		{name: "void placed", m: mv(TypeVoid, nil), cur: held(StatusPlaced, distLoc),
			wantStatus: StatusVoid, wantOwner: Owner{Type: OwnerTrash, ID: dist, OrgID: dist}, wantQty: -1},
		{name: "void twice", m: mv(TypeVoid, nil), cur: held(StatusVoid, Owner{Type: OwnerTrash, ID: dist, OrgID: dist}), wantErr: ErrTransitionNotAllowed},
		{name: "void used", m: mv(TypeVoid, nil), cur: held(StatusUsed, service), wantErr: ErrTransitionNotAllowed},
		{name: "void to other trash", m: mv(TypeVoid, &Owner{Type: OwnerTrash, ID: center}), cur: held(StatusPlaced, distLoc), wantErr: ErrOwnerNotAllowed},
		{name: "external_outbound", m: mv(TypeExternalOutbound, nil), cur: held(StatusAvailable, centerLoc),
			wantStatus: StatusUsed, wantOwner: Owner{Type: OwnerTrash, ID: center, OrgID: center}, wantQty: -1},
		{name: "external_outbound in transit", m: mv(TypeExternalOutbound, nil), cur: held(StatusInTransit, distOrg), wantErr: ErrTransitionNotAllowed},
		// owner check
		{name: "stale From", m: Movement{Type: TypeTransferOut, From: ptr(centerLoc), To: ptr(dealerOrg), RefType: "doc", RefID: 7},
			cur: held(StatusAvailable, distLoc), wantErr: ErrOwnerMismatch},
		{name: "From on a label", m: Movement{Type: TypeEntry, From: ptr(centerLoc), To: ptr(centerLoc), RefType: "doc", RefID: 7},
			cur: label(StatusPrinted), wantErr: ErrOwnerMismatch},
		{name: "matching From", m: Movement{Type: TypeTransferOut, From: ptr(distLoc), To: ptr(dealerOrg), RefType: "doc", RefID: 7},
			cur: held(StatusAvailable, distLoc), wantStatus: StatusInTransit, wantOwner: dealerOrg, wantQty: -1},
		{name: "meters on a piece", m: Movement{Type: TypeTransferOut, To: ptr(dealerOrg), Centimeters: 1, RefType: "doc", RefID: 7},
			cur: held(StatusAvailable, distLoc), wantErr: ErrInvalidMovement},
		{name: "reclassification is not postable here", m: mv("reclassification", nil), cur: held(StatusAvailable, distLoc), wantErr: ErrTransitionNotAllowed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			plan, err := planSerial(c.m, c.cur, c.prev)
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("err = %v, want %v", err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if plan.toStatus != c.wantStatus || plan.toOwner != c.wantOwner ||
				plan.quantityDelta != c.wantQty || plan.metersDelta != c.wantCm {
				t.Fatalf("plan = %+v, want status %s owner %+v qty %d cm %d",
					plan, c.wantStatus, c.wantOwner, c.wantQty, c.wantCm)
			}
		})
	}
}

// Every postable type has a serial rule; placement and partial
// consumption have no fixed rule (decision 1).
func TestRuleCoverage(t *testing.T) {
	for _, typ := range PostableTypes {
		if _, ok := serialRules[typ]; !ok {
			t.Errorf("no serial rule for %s", typ)
		}
		_, fixed := fixedRules[typ]
		if want := typ != TypePlacement && typ != TypePartialConsumption; fixed != want {
			t.Errorf("fixed rule for %s = %v, want %v", typ, fixed, want)
		}
		if name := EventName(typ); !events.IsKnownEvent(name) {
			t.Errorf("event %q for %s not in the catalog", name, typ)
		}
	}
	if !events.IsKnownEvent(EventName("reclassification")) {
		t.Error("reclassification event missing")
	}
}

func TestFixedTransitions(t *testing.T) {
	type tc struct {
		name       string
		m          Movement
		status     Status
		wantErr    error
		wantOwner  Owner
		wantDelta  int32
		wantStatus Status
	}
	q := func(t MovementType, from, to *Owner, n int32) Movement {
		return Movement{Type: t, From: from, To: to, Quantity: n, RefType: "doc", RefID: 7}
	}
	cases := []tc{
		{name: "entry printed", m: q(TypeEntry, nil, ptr(centerLoc), 10), status: StatusPrinted, wantOwner: centerLoc, wantDelta: 10, wantStatus: StatusAvailable},
		{name: "entry again", m: q(TypeEntry, nil, ptr(centerLoc), 5), status: StatusAvailable, wantOwner: centerLoc, wantDelta: 5},
		{name: "entry void", m: q(TypeEntry, nil, ptr(centerLoc), 5), status: StatusVoid, wantErr: ErrTransitionNotAllowed},
		{name: "entry no target", m: q(TypeEntry, ptr(centerLoc), nil, 5), status: StatusAvailable, wantErr: ErrInvalidMovement},
		{name: "entry zero", m: q(TypeEntry, nil, ptr(centerLoc), 0), status: StatusAvailable, wantErr: ErrInvalidMovement},
		{name: "entry negative", m: q(TypeEntry, nil, ptr(centerLoc), -1), status: StatusAvailable, wantErr: ErrInvalidMovement},
		{name: "placement", m: q(TypePlacement, ptr(centerLoc), ptr(centerLoc), 1), status: StatusAvailable, wantErr: ErrTransitionNotAllowed},
		{name: "partial", m: q(TypePartialConsumption, ptr(centerLoc), nil, 1), status: StatusAvailable, wantErr: ErrTransitionNotAllowed},
		{name: "transfer_out", m: q(TypeTransferOut, ptr(centerLoc), nil, 3), status: StatusAvailable, wantOwner: centerLoc, wantDelta: -3},
		{name: "transfer_out no source", m: q(TypeTransferOut, nil, ptr(distOrg), 3), status: StatusAvailable, wantErr: ErrInvalidMovement},
		{name: "transfer_out printed", m: q(TypeTransferOut, ptr(centerLoc), nil, 3), status: StatusPrinted, wantErr: ErrTransitionNotAllowed},
		{name: "transfer_in", m: q(TypeTransferIn, nil, ptr(distLoc), 3), status: StatusAvailable, wantOwner: distLoc, wantDelta: 3},
		{name: "transfer_cancel_restore", m: q(TypeTransferCancelRestore, nil, ptr(centerLoc), 3), status: StatusAvailable, wantOwner: centerLoc, wantDelta: 3},
		{name: "order_out", m: q(TypeOrderOut, ptr(distOrg), nil, 2), status: StatusAvailable, wantOwner: distOrg, wantDelta: -2},
		{name: "received", m: q(TypeReceived, nil, ptr(dealerOrg), 2), status: StatusAvailable, wantOwner: dealerOrg, wantDelta: 2},
		{name: "order_cancel_restore", m: q(TypeOrderCancelRestore, nil, ptr(distOrg), 2), status: StatusAvailable, wantOwner: distOrg, wantDelta: 2},
		{name: "consumption", m: q(TypeConsumption, ptr(dealerOrg), nil, 1), status: StatusAvailable, wantOwner: dealerOrg, wantDelta: -1},
		{name: "consumption to service holding", m: q(TypeReturn, nil, ptr(service), 1), status: StatusAvailable, wantErr: ErrOwnerNotAllowed},
		{name: "return", m: q(TypeReturn, nil, ptr(dealerOrg), 1), status: StatusAvailable, wantOwner: dealerOrg, wantDelta: 1},
		{name: "count +2", m: q(TypeCountAdjustment, nil, ptr(distLoc), 2), status: StatusAvailable, wantOwner: distLoc, wantDelta: 2},
		{name: "count -2", m: q(TypeCountAdjustment, ptr(distLoc), nil, -2), status: StatusAvailable, wantOwner: distLoc, wantDelta: -2},
		{name: "count -2 without source", m: q(TypeCountAdjustment, nil, ptr(distLoc), -2), status: StatusAvailable, wantErr: ErrInvalidMovement},
		{name: "count zero", m: q(TypeCountAdjustment, ptr(distLoc), ptr(distLoc), 0), status: StatusAvailable, wantErr: ErrInvalidMovement},
		{name: "void", m: q(TypeVoid, ptr(distLoc), nil, 1), status: StatusAvailable, wantOwner: distLoc, wantDelta: -1},
		{name: "external_outbound", m: q(TypeExternalOutbound, ptr(centerLoc), nil, 4), status: StatusAvailable, wantOwner: centerLoc, wantDelta: -4},
		{name: "meters", m: Movement{Type: TypeConsumption, From: ptr(dealerOrg), Quantity: 1, Centimeters: 5}, status: StatusAvailable, wantErr: ErrInvalidMovement},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			plan, err := planFixed(c.m, c.status)
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("err = %v, want %v", err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if plan.owner != c.wantOwner || plan.delta != c.wantDelta || plan.setStatus != c.wantStatus {
				t.Fatalf("plan = %+v, want owner %+v delta %d status %q", plan, c.wantOwner, c.wantDelta, c.wantStatus)
			}
		})
	}
}

func TestIdempotencyKey(t *testing.T) {
	got, err := IdempotencyKey("api", "service_item", 42, TypeConsumption, "OLX-0001")
	if err != nil || got != "api:service_item:42:consumption:OLX-0001" {
		t.Fatalf("key = %q, %v", got, err)
	}
	bad := []struct {
		source, refType string
		refID           int64
		barcode         string
	}{
		{"", "doc", 1, "B"}, {"api", "", 1, "B"}, {"api", "doc", 0, "B"}, {"api", "doc", 1, ""},
		{"a:b", "doc", 1, "B"}, {"api", "d:c", 1, "B"},
		{"api", "doc", 1, fill(120)},
	}
	for _, b := range bad {
		if _, err := IdempotencyKey(b.source, b.refType, b.refID, TypeEntry, b.barcode); !errors.Is(err, ErrInvalidMovement) {
			t.Errorf("IdempotencyKey(%q,%q,%d,%q) err = %v", b.source, b.refType, b.refID, b.barcode, err)
		}
	}
}

func fill(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'x'
	}
	return string(b)
}

func TestMeters(t *testing.T) {
	for in, want := range map[string]int64{"15": 1500, "5.5": 550, "4.25": 425, "-1": -100, "0.05": 5, "6.": 600} {
		got, err := ParseMeters(in)
		if err != nil || got != want {
			t.Errorf("ParseMeters(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", "1.234", "abc", "--1", ".5", "1.-5", "+1"} {
		if _, err := ParseMeters(in); err == nil {
			t.Errorf("ParseMeters(%q) accepted", in)
		}
	}
	if FormatMeters(600) != "6.00" || FormatMeters(-425) != "-4.25" {
		t.Fatalf("FormatMeters: %s %s", FormatMeters(600), FormatMeters(-425))
	}
	for _, cm := range []int64{0, 1, 600, 1500, -425, 99999999} {
		got, err := numericToCm(cmToNumeric(cm))
		if err != nil || got != cm {
			t.Errorf("roundtrip %d = %d, %v", cm, got, err)
		}
	}
	var n = cmToNumeric(15)
	n.Exp = -3 // 0.015 m
	if _, err := numericToCm(n); err == nil {
		t.Error("three decimals accepted")
	}
	n.Exp = 1 // 150 * 10 / ... = 1.5 * 10^2 m
	if got, _ := numericToCm(n); got != 15000 {
		t.Errorf("exp 1: %d", got)
	}
}

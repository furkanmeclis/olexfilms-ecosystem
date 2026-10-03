package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	customersusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/customers/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/search/indexsync"
	servicesusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	warrantyusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	"github.com/google/uuid"
)

// memIndex is an in-memory stand-in for Meilisearch behind the module
// lists (searchengine.ListFinder). It evaluates the filter expressions the
// lists build (field = n, field = "s", field IN [..], AND) on the
// documents the adapters produce and refuses fields the spec does not
// declare filterable (Meilisearch would reject them). leaky ignores the
// filter: the Postgres reload must still keep the scope.
type memIndex struct {
	mu      sync.Mutex
	reg     *searchengine.Registry
	docs    map[string]map[string]searchengine.Document
	leaky   bool
	queries map[string]int
	t       *testing.T
}

func newMemIndex(t *testing.T, reg *searchengine.Registry) *memIndex {
	return &memIndex{t: t, reg: reg, docs: map[string]map[string]searchengine.Document{}, queries: map[string]int{}}
}

func (m *memIndex) Enabled() bool { return true }

// reindex loads every adapter's ListAll (what cmd/search-reindex does).
func (m *memIndex) reindex(ctx context.Context) map[string]int {
	m.t.Helper()
	counts := map[string]int{}
	for _, id := range m.reg.SpecIDs() {
		a, _ := m.reg.Get(id)
		docs, err := a.ListAll(ctx)
		if err != nil {
			m.t.Fatalf("reindex %s: %v", id, err)
		}
		m.mu.Lock()
		m.docs[id] = map[string]searchengine.Document{}
		for _, d := range docs {
			m.docs[id][d.ID] = d
		}
		m.mu.Unlock()
		counts[id] = len(docs)
	}
	return counts
}

// EnqueueUpsert applies an index refresh synchronously (indexsync.Indexer).
func (m *memIndex) EnqueueUpsert(ctx context.Context, spec, id string) {
	a, err := m.reg.Get(spec)
	if err != nil {
		m.t.Errorf("upsert unknown spec %s", spec)
		return
	}
	doc, err := a.Document(ctx, id)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.docs[spec] == nil {
		m.docs[spec] = map[string]searchengine.Document{}
	}
	if err != nil {
		delete(m.docs[spec], id)
		return
	}
	m.docs[spec][id] = doc
}

func (m *memIndex) doc(spec, id string) (searchengine.Document, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.docs[spec][id]
	return d, ok
}

func (m *memIndex) SearchIDs(_ context.Context, spec, q, filter string, limit, offset int) ([]string, int64, error) {
	a, err := m.reg.Get(spec)
	if err != nil {
		return nil, 0, err
	}
	allowed := a.Spec().Filterable
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queries[spec]++
	q = strings.ToLower(strings.TrimSpace(q))
	var ids []string
	for id, d := range m.docs[spec] {
		if q != "" && !docMatches(d, q) {
			continue
		}
		if !m.leaky && !m.filterMatches(d, filter, allowed) {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	total := int64(len(ids))
	if offset > len(ids) {
		offset = len(ids)
	}
	ids = ids[offset:]
	if limit > 0 && limit < len(ids) {
		ids = ids[:limit]
	}
	return ids, total, nil
}

func docMatches(d searchengine.Document, q string) bool {
	for _, s := range append([]string{d.Title, d.Subtitle}, d.Keywords...) {
		if strings.Contains(strings.ToLower(s), q) {
			return true
		}
	}
	return false
}

func (m *memIndex) filterMatches(d searchengine.Document, filter string, allowed []string) bool {
	if strings.TrimSpace(filter) == "" {
		return true
	}
	raw, _ := json.Marshal(d)
	var fields map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber() // ids stay "1234567", not "1.234567e+06"
	_ = dec.Decode(&fields)
	for _, part := range strings.Split(filter, " AND ") {
		var field, op, val string
		if i := strings.Index(part, " IN "); i > 0 {
			field, op, val = part[:i], "in", part[i+4:]
		} else if i := strings.Index(part, " = "); i > 0 {
			field, op, val = part[:i], "eq", part[i+3:]
		} else {
			m.t.Errorf("unparsable filter part %q", part)
			return false
		}
		if !slices.Contains(allowed, field) {
			m.t.Errorf("filter on %q, not filterable (%v)", field, allowed)
			return false
		}
		var want []string
		if op == "in" {
			for _, v := range strings.Split(strings.Trim(val, "[]"), ",") {
				if v = strings.TrimSpace(v); v != "" {
					want = append(want, v)
				}
			}
		} else {
			if s, err := strconv.Unquote(val); err == nil {
				val = s
			}
			want = []string{val}
		}
		var have []string
		switch v := fields[field].(type) {
		case []any:
			for _, x := range v {
				have = append(have, fmt.Sprint(x))
			}
		case nil:
		default:
			have = []string{fmt.Sprint(v)}
		}
		hit := false
		for _, h := range have {
			if slices.Contains(want, h) {
				hit = true
			}
		}
		if !hit {
			return false
		}
	}
	return true
}

// outboxEventsOf decodes the outbox rows of one entity the way the outbox
// publisher hands them to the bus.
func (it *itest) outboxEventsOf(name, entityUUID string) []events.Event {
	it.t.Helper()
	rows, err := it.pool.Query(context.Background(), `SELECT payload FROM outbox_events
		WHERE event_name = $1 AND payload->>'entity_uuid' = $2 ORDER BY id`, name, entityUUID)
	if err != nil {
		it.t.Fatal(err)
	}
	defer rows.Close()
	var out []events.Event
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			it.t.Fatal(err)
		}
		var env struct {
			EventID    uuid.UUID      `json:"event_id"`
			EntityType string         `json:"entity_type"`
			EntityID   *int64         `json:"entity_id"`
			EntityUUID *uuid.UUID     `json:"entity_uuid"`
			Data       map[string]any `json:"data"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			it.t.Fatal(err)
		}
		out = append(out, events.Event{
			Name: name, EventID: env.EventID, EntityType: env.EntityType, EntityID: env.EntityID,
			EntityUUID: env.EntityUUID, Payload: env.Data, OccurredAt: time.Now().UTC(),
		})
	}
	if len(out) == 0 {
		it.t.Fatalf("no %s outbox event for %s", name, entityUUID)
	}
	return out
}

func listUUIDs(t *testing.T, env envelope) map[string]bool {
	t.Helper()
	var page struct {
		Items []struct {
			UUID string `json:"uuid"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal(env.Data, &page); err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, i := range page.Items {
		out[i.UUID] = true
	}
	return out
}

// TEC-209 acceptance: the services, warranties and vehicles indexes.
//  1. Reindex (cmd/search-reindex: every adapter's ListAll) loads all
//     records, Glorian's included.
//  2. q goes to the index; the tenant scope + brand filter never returns
//     another organization's (or brand's) record, also when the index
//     ignores the filter (the Postgres reload keeps the scope).
//  3. A record change reaches the index through the outbox: the events of
//     a new service, a vehicle edit and a warranty void refresh the
//     documents.
func TestIntegrationSearchRecordIndexes(t *testing.T) {
	finder := &lateFinder{}
	it := newIntegrationWithDeps(t, nil, func(d *Deps) { d.SearchFinder = finder })
	ctx := context.Background()
	mem := newMemIndex(t, searchengine.NewRegistry(
		servicesusecase.NewSearchAdapter(it.q),
		warrantyusecase.NewSearchAdapter(it.q),
		customersusecase.NewVehicleSearchAdapter(it.q),
	))
	finder.m = mem

	sfx := it.suffix[len(it.suffix)-6:]
	center := it.brandCenter("olex")
	dist := it.org("t209-dist", "distributor", center)
	dealerA := it.org("t209-a", "dealer", dist)
	dealerB := it.org("t209-b", "dealer", dist)
	gCenter := it.brandCenter("glorian")
	gDealer := it.org("t209-g", "dealer", gCenter)

	p := it.product(center, "T209")
	it.setWarrantyMonths(p, 12)
	custA, vehA := it.svcCustomer(dealerA, "t209-cust-a", "34T209"+sfx)
	custB, vehB := it.svcCustomer(dealerB, "t209-cust-b", "06T209"+sfx)
	gCust, gVeh := it.svcCustomer(gDealer, "t209-cust-g", "35T209"+sfx)
	wA := it.warrantyFor(dealerA, center, p, custA, vehA, 209)
	wB := it.warrantyFor(dealerB, center, p, custB, vehB, 210)
	svcG := it.directService(gDealer, gCust, gVeh, 209211)
	var svcA, svcB uuid.UUID
	if err := it.pool.QueryRow(ctx, `SELECT uuid FROM services WHERE id = $1`, wA.ServiceID).Scan(&svcA); err != nil {
		t.Fatal(err)
	}
	if err := it.pool.QueryRow(ctx, `SELECT uuid FROM services WHERE id = $1`, wB.ServiceID).Scan(&svcB); err != nil {
		t.Fatal(err)
	}

	login := func(org db.Organization, name, role string) (db.User, string) {
		u, pw := it.user(name)
		it.member(org, u, role)
		return u, it.loginOrg(u, pw, org)
	}
	_, tokA := login(dealerA, "t209-owner-a", "owner")
	centerUser, tokCenter := login(center, "t209-center", "staff")

	// 1. Reindex loads every record.
	counts := mem.reindex(ctx)
	for spec, ids := range map[string][]string{
		searchengine.SpecServices:   {svcA.String(), svcB.String(), svcG.Uuid.String()},
		searchengine.SpecWarranties: {wA.Uuid.String(), wB.Uuid.String()},
		searchengine.SpecVehicles:   {vehA.Uuid.String(), vehB.Uuid.String(), gVeh.Uuid.String()},
	} {
		for _, id := range ids {
			if _, ok := mem.doc(spec, id); !ok {
				t.Fatalf("reindex %s (%d docs) misses %s", spec, counts[spec], id)
			}
		}
	}
	if d, _ := mem.doc(searchengine.SpecWarranties, wA.Uuid.String()); d.Title != wA.PublicCode {
		t.Fatalf("warranty doc = %+v", d)
	}

	// 2. Scope + brand, strict and leaky index.
	get := func(path, tok string) map[string]bool {
		t.Helper()
		return listUUIDs(t, it.custDo("GET", path, tok, nil, http.StatusOK))
	}
	token := url.QueryEscape("T209" + sfx)
	// directService writes no plate snapshot: the services share the
	// service number prefix (T186-<suffix>-...).
	svcToken := url.QueryEscape("T186-" + it.suffix[len(it.suffix)-10:])
	for _, leaky := range []bool{false, true} {
		mem.leaky = leaky
		label := fmt.Sprintf("leaky=%v", leaky)
		got := get("/v1/services?q="+svcToken, tokA)
		if !got[svcA.String()] || got[svcB.String()] || got[svcG.Uuid.String()] {
			t.Fatalf("%s dealer A services = %v", label, got)
		}
		got = get("/v1/services?q="+svcToken, tokCenter)
		if !got[svcA.String()] || !got[svcB.String()] || got[svcG.Uuid.String()] {
			t.Fatalf("%s center services (brand filter) = %v", label, got)
		}
		got = get("/v1/vehicles?q="+token, tokA)
		if !got[vehA.Uuid.String()] || got[vehB.Uuid.String()] || got[gVeh.Uuid.String()] {
			t.Fatalf("%s dealer A vehicles = %v", label, got)
		}
		got = get("/v1/warranties?q="+url.QueryEscape(wB.PublicCode), tokA)
		if len(got) != 0 {
			t.Fatalf("%s dealer A found dealer B's warranty: %v", label, got)
		}
		got = get("/v1/warranties?q="+url.QueryEscape(wA.PublicCode), tokA)
		if !got[wA.Uuid.String()] || len(got) != 1 {
			t.Fatalf("%s dealer A warranty by public code = %v", label, got)
		}
	}
	mem.leaky = false
	for _, spec := range []string{searchengine.SpecServices, searchengine.SpecVehicles, searchengine.SpecWarranties} {
		if mem.queries[spec] == 0 {
			t.Fatalf("%s list did not query the index", spec)
		}
	}

	// 3. Outbox -> index.
	sync := indexsync.New(it.q, mem, nil)
	deliver := func(name, entity string) {
		t.Helper()
		for _, ev := range it.outboxEventsOf(name, entity) {
			var err error
			switch {
			case strings.HasPrefix(name, "service."):
				err = sync.HandleService(ctx, ev)
			case strings.HasPrefix(name, "warranty."):
				err = sync.HandleWarranty(ctx, ev)
			case strings.HasPrefix(name, "vehicle."):
				err = sync.HandleVehicle(ctx, ev)
			}
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
	}

	// 3a. A new service enters the index from service.created.
	created := decodeData[serviceView](t, it.custDo("POST", "/v1/services", tokA, map[string]any{
		"customer_uuid": custA.Uuid.String(), "vehicle_uuid": vehA.Uuid.String(),
	}, http.StatusCreated))
	if _, ok := mem.doc(searchengine.SpecServices, created.UUID); ok {
		t.Fatal("service indexed before its event")
	}
	deliver(events.ServiceCreated, created.UUID)
	if d, ok := mem.doc(searchengine.SpecServices, created.UUID); !ok || d.Title != created.ServiceNo ||
		!slices.Equal(d.OrganizationIDs, []int64{dealerA.ID}) {
		t.Fatalf("new service doc = %+v (%v)", d, ok)
	}
	if got := get("/v1/services?q="+url.QueryEscape(created.ServiceNo), tokA); !got[created.UUID] {
		t.Fatalf("new service not found by number: %v", got)
	}

	// 3b. A vehicle edit (VIN) refreshes the vehicle document.
	vin := "WVWZZZ1JZ" + it.suffix[len(it.suffix)-8:]
	it.custDo("PATCH", "/v1/vehicles/"+vehA.Uuid.String(), tokA, map[string]any{"vin": vin}, http.StatusOK)
	if got := get("/v1/vehicles?q="+vin, tokA); got[vehA.Uuid.String()] {
		t.Fatal("index answered the new VIN before the outbox event")
	}
	deliver(events.VehicleUpdated, vehA.Uuid.String())
	if got := get("/v1/vehicles?q="+vin, tokA); !got[vehA.Uuid.String()] {
		t.Fatalf("vehicle not found by the new VIN after vehicle.updated: %v", got)
	}

	// 3c. A warranty void refreshes the status filter.
	it.stepUp(centerUser.Uuid)
	it.custDo("POST", "/v1/warranties/"+wA.Uuid.String()+"/void", tokCenter,
		map[string]any{"reason": "TEC-209 index test"}, http.StatusOK)
	deliver(events.WarrantyVoided, wA.Uuid.String())
	if d, _ := mem.doc(searchengine.SpecWarranties, wA.Uuid.String()); d.Status != "void" {
		t.Fatalf("warranty doc after void = %+v", d)
	}
	if got := get("/v1/warranties?status=void&q="+url.QueryEscape(wA.PublicCode), tokA); !got[wA.Uuid.String()] {
		t.Fatalf("voided warranty by status filter = %v", got)
	}
}

// lateFinder lets the test build the in-memory index after the server
// (the adapters need the server's queries).
type lateFinder struct{ m *memIndex }

func (l *lateFinder) Enabled() bool { return l.m != nil }

func (l *lateFinder) SearchIDs(ctx context.Context, spec, q, filter string, limit, offset int) ([]string, int64, error) {
	return l.m.SearchIDs(ctx, spec, q, filter, limit, offset)
}

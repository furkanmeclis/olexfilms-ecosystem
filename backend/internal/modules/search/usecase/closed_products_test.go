package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	"github.com/google/uuid"
)

type fakeClosed struct {
	ids   []uuid.UUID
	err   error
	calls int
}

func (f *fakeClosed) ListDealerClosedProductUUIDs(_ context.Context, in []uuid.UUID) ([]uuid.UUID, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	var out []uuid.UUID
	for _, id := range in {
		for _, c := range f.ids {
			if id == c {
				out = append(out, id)
			}
		}
	}
	return out, nil
}

func hitIDs(hits []searchengine.Hit) []string {
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.ID)
	}
	return out
}

// TEC-1061: outside the center (no tenant here) the product hits closed to
// dealer orders are dropped, other specs stay, the limit still holds.
func TestVisibleHitsDropsClosedProducts(t *testing.T) {
	open, shut := uuid.New(), uuid.New()
	svc := New(nil, nil, nil, nil)
	f := &fakeClosed{ids: []uuid.UUID{shut}}
	svc.closed = f
	hits := []searchengine.Hit{
		{Spec: searchengine.SpecProducts, ID: shut.String()},
		{Spec: searchengine.SpecProducts, ID: open.String()},
		{Spec: "roles", ID: "r1"},
		{Spec: "roles", ID: "r2"},
	}
	got := hitIDs(svc.VisibleHits(context.Background(), hits, 2))
	if len(got) != 2 || got[0] != open.String() || got[1] != "r1" {
		t.Fatalf("hits = %v", got)
	}
	if f.calls != 1 {
		t.Fatalf("lookups = %d", f.calls)
	}
}

// A failed or missing lookup hides every product hit (fail closed).
func TestVisibleHitsFailsClosed(t *testing.T) {
	hits := []searchengine.Hit{
		{Spec: searchengine.SpecProducts, ID: uuid.NewString()},
		{Spec: "roles", ID: "r1"},
	}
	svc := New(nil, nil, nil, nil)
	svc.closed = &fakeClosed{err: errors.New("db down")}
	if got := hitIDs(svc.VisibleHits(context.Background(), hits, 10)); len(got) != 1 || got[0] != "r1" {
		t.Fatalf("error: hits = %v", got)
	}
	svc.closed = nil
	if got := hitIDs(svc.VisibleHits(context.Background(), hits, 10)); len(got) != 1 || got[0] != "r1" {
		t.Fatalf("nil lookup: hits = %v", got)
	}
}

// Without product hits nothing is looked up; only the limit applies.
func TestVisibleHitsSkipsLookupWithoutProducts(t *testing.T) {
	svc := New(nil, nil, nil, nil)
	f := &fakeClosed{}
	svc.closed = f
	hits := []searchengine.Hit{{Spec: "roles", ID: "r1"}, {Spec: "roles", ID: "r2"}}
	if got := svc.VisibleHits(context.Background(), hits, 1); len(got) != 1 {
		t.Fatalf("hits = %v", got)
	}
	if f.calls != 0 {
		t.Fatalf("lookups = %d", f.calls)
	}
}

func TestFetchLimitOverFetchesProducts(t *testing.T) {
	if got := fetchLimit([]string{"roles"}, 5); got != 5 {
		t.Fatalf("roles = %d", got)
	}
	if got := fetchLimit([]string{"roles", searchengine.SpecProducts}, 5); got != 10 {
		t.Fatalf("products = %d", got)
	}
	if got := fetchLimit([]string{searchengine.SpecProducts}, 0); got != 40 {
		t.Fatalf("default = %d", got)
	}
}

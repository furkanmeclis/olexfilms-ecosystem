package searchengine

import (
	"testing"

	"github.com/google/uuid"
)

func TestFilterString(t *testing.T) {
	f := (&Filter{}).Eq("brand_ids", 2).Eq("skip", 0).In("organization_ids", []int64{5, 7}).In("none", nil).EqString("status", ` active `)
	if got := f.String(); got != `brand_ids = 2 AND organization_ids IN [5, 7] AND status = "active"` {
		t.Fatalf("filter = %q", got)
	}
	if got := (&Filter{}).In("organization_ids", []int64{}).String(); got != "organization_ids IN []" {
		t.Fatalf("empty IN = %q", got)
	}
}

func TestParseUUIDsAndReorder(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	ids, rank := ParseUUIDs([]string{b.String(), "bad", a.String(), b.String(), c.String()})
	if len(ids) != 3 || ids[0] != b || ids[1] != a || ids[2] != c {
		t.Fatalf("ids = %v", ids)
	}
	// Postgres returns its own order and drops c (out of scope).
	rows := []uuid.UUID{a, b}
	got := Reorder(rows, func(u uuid.UUID) uuid.UUID { return u }, rank)
	if len(got) != 2 || got[0] != b || got[1] != a {
		t.Fatalf("reorder = %v", got)
	}
}

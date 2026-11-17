package credits

import (
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5/pgtype"
)

// Only a serial unit without meters carries credits (TEC-1221).
func TestApplies(t *testing.T) {
	var meters pgtype.Numeric
	if err := meters.Scan("50.00"); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		u    db.Unit
		want bool
	}{
		{"serial piece", db.Unit{UnitKind: "serial"}, true},
		{"roll", db.Unit{UnitKind: "serial", InitialMeters: meters}, false},
		{"fixed barcode", db.Unit{UnitKind: "fixed"}, false},
	}
	for _, c := range cases {
		if got := Applies(c.u); got != c.want {
			t.Errorf("%s: Applies = %v, want %v", c.name, got, c.want)
		}
	}
	if !(Balance{Remaining: 0}).Exhausted() || (Balance{Remaining: 1}).Exhausted() {
		t.Error("Exhausted")
	}
}

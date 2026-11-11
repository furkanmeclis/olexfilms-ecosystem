package usecase

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
)

func TestCallerIDLabel(t *testing.T) {
	cases := []struct{ name, surname, company, want string }{
		{"Ahmet", "Yılmaz", "", "Ahmet Yılmaz"},
		{" Ahmet\t", "Yılmaz\n", "", "Ahmet Yılmaz"},
		{"Ahmet", "Yılmaz", "ABC Oto Ltd", "Ahmet Yılmaz (ABC Oto Ltd)"},
		{"", "", "ABC Oto Ltd", "ABC Oto Ltd"},
		{"ABC Oto", "", "abc oto", "ABC Oto"},
		{"", "", "", ""},
	}
	for _, c := range cases {
		if got := callerIDLabel(c.name, c.surname, c.company); got != c.want {
			t.Errorf("callerIDLabel(%q, %q, %q) = %q, want %q", c.name, c.surname, c.company, got, c.want)
		}
	}
	long := callerIDLabel(strings.Repeat("ş", 200), "", "")
	if n := utf8.RuneCountInString(long); n != callerIDLabelMax {
		t.Errorf("long label has %d characters, want %d", n, callerIDLabelMax)
	}
	if !strings.HasSuffix(long, "…") {
		t.Errorf("long label is not marked as cut: %q", long)
	}
}

func TestBuildCallerIDSnapshot(t *testing.T) {
	rows := []db.ListCallerIDEntriesRow{
		{Phone: "+905321112233", Name: "Ahmet", Surname: "Yılmaz"},
		{Phone: "+905339998877", Name: "", Surname: ""}, // no label: skipped
		{Phone: "+905445556677", Name: "Ayşe", Surname: "Kaya", CompanyName: "Kaya Oto"},
	}
	first := buildCallerIDSnapshot(rows, "", 10)
	if first.Unchanged || first.Truncated || first.Count != 2 || len(first.Entries) != 2 {
		t.Fatalf("first snapshot: %+v", first)
	}
	if first.Entries[1] != (CallerIDEntry{Phone: "+905445556677", Label: "Ayşe Kaya (Kaya Oto)"}) {
		t.Errorf("entry: %+v", first.Entries[1])
	}
	if len(first.Version) != 32 {
		t.Errorf("version %q", first.Version)
	}

	// The version the device holds: no entries travel again.
	same := buildCallerIDSnapshot(rows, first.Version, 10)
	if !same.Unchanged || same.Count != 2 || len(same.Entries) != 0 || same.Entries == nil || same.Version != first.Version {
		t.Errorf("unchanged snapshot: %+v", same)
	}

	// A changed label, a removed customer and a stale version all resend.
	rows[0].Surname = "Yilmaz"
	changed := buildCallerIDSnapshot(rows, first.Version, 10)
	if changed.Unchanged || changed.Version == first.Version || len(changed.Entries) != 2 {
		t.Errorf("changed snapshot: %+v", changed)
	}
	removed := buildCallerIDSnapshot(rows[:1], changed.Version, 10)
	if removed.Unchanged || removed.Count != 1 {
		t.Errorf("removed snapshot: %+v", removed)
	}

	// Over the cap: cut and reported.
	cut := buildCallerIDSnapshot(rows, "", 1)
	if !cut.Truncated || cut.Count != 1 {
		t.Errorf("truncated snapshot: %+v", cut)
	}

	empty := buildCallerIDSnapshot(nil, "", 10)
	if empty.Count != 0 || empty.Entries == nil || empty.Version == "" {
		t.Errorf("empty snapshot: %+v", empty)
	}
}

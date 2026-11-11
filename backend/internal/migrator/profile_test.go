package migrator

import (
	"context"
	"errors"
	"testing"
)

type namedStep struct{ name string }

func (s namedStep) Name() string { return s.name }
func (s namedStep) Run(context.Context, Sources, *Target, *Mapper) (StepResult, error) {
	return StepResult{}, nil
}

// --profile=glorian is refused before the database is touched (Pool is nil).
func TestRunGlorianProfileRefused(t *testing.T) {
	r := &Runner{}
	_, err := r.Run(context.Background(), Options{Profile: "glorian"})
	if !errors.Is(err, ErrProfileDisabled) {
		t.Fatalf("Run(glorian) = %v, want ErrProfileDisabled", err)
	}
	if _, err := r.Run(context.Background(), Options{Profile: "nope"}); !errors.Is(err, ErrUnknownProfile) {
		t.Fatalf("Run(nope) = %v, want ErrUnknownProfile", err)
	}
	if _, ok := Profiles()["glorian"]; !ok {
		t.Fatal("glorian profile must stay registered (K2)")
	}
	if p, err := Lookup(Profiles(), "olex"); err != nil || !p.Enabled {
		t.Fatalf("olex profile = %+v, %v", p, err)
	}
}

func TestRunRejectsBadMode(t *testing.T) {
	r := &Runner{}
	if _, err := r.Run(context.Background(), Options{Profile: "olex", Mode: "partial"}); err == nil {
		t.Fatal("Run(mode=partial) succeeded")
	}
	for _, m := range []string{"full", "delta"} {
		if _, err := ParseMode(m); err != nil {
			t.Errorf("ParseMode(%s) = %v", m, err)
		}
	}
}

func TestSelectSteps(t *testing.T) {
	p := Profile{Name: "x", Enabled: true, Steps: func() []Step {
		return []Step{namedStep{"a"}, namedStep{"b"}, namedStep{"c"}}
	}}
	all, err := SelectSteps(p, nil)
	if err != nil || len(all) != 3 {
		t.Fatalf("all = %v, %v", all, err)
	}
	got, err := SelectSteps(p, []string{"c", " a "})
	if err != nil || len(got) != 2 || got[0].Name() != "a" || got[1].Name() != "c" {
		t.Fatalf("subset = %v, %v (want profile order a, c)", got, err)
	}
	if _, err := SelectSteps(p, []string{"a", "zzz"}); !errors.Is(err, ErrUnknownStep) {
		t.Fatalf("unknown step = %v", err)
	}
}

func TestChecksum(t *testing.T) {
	if Checksum("a", "bc") == Checksum("ab", "c") {
		t.Error("field boundaries must change the checksum")
	}
	if a, b := Checksum(1, nil, "x"), Checksum(1, nil, "x"); a != b {
		t.Error("checksum must be deterministic")
	}
	if Checksum(nil) == Checksum("") {
		t.Error("nil and empty string must differ")
	}
	if n := len(Checksum("x")); n != 64 {
		t.Errorf("checksum length = %d, want 64 (VARCHAR(64))", n)
	}
}

// TEC-1202: the Olex profile makes the legacy FLORYA warehouse the center's
// "Merkez Depo" with code CENTRAL; AFM Florya itself is imported as a normal
// dealer (OrganizationsStep has no service point merge).
func TestOlexProfilePrimaryWarehouse(t *testing.T) {
	var found bool
	for _, st := range olexSteps() {
		w, ok := st.(WarehousesStep)
		if !ok {
			continue
		}
		found = true
		if w.PrimaryCode != "FLORYA" || w.PrimaryNewCode != "CENTRAL" || w.PrimaryName != "Merkez Depo" {
			t.Errorf("primary warehouse = %q -> %q %q, want FLORYA -> CENTRAL / Merkez Depo",
				w.PrimaryCode, w.PrimaryNewCode, w.PrimaryName)
		}
		if got := w.targetCode("florya"); got != "CENTRAL" {
			t.Errorf("targetCode(florya) = %q, want CENTRAL", got)
		}
		if got := w.targetCode("other-wh"); got != "OTHER_WH" {
			t.Errorf("targetCode(other-wh) = %q, want OTHER_WH", got)
		}
	}
	if !found {
		t.Fatal("no warehouses step in the olex profile")
	}
}

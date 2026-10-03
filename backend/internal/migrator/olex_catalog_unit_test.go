package migrator

import (
	"database/sql"
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator/source"
)

// The TEC-256 step queries stay inside the read-only guard, in full and
// delta form.
func TestCatalogQueriesPassGuard(t *testing.T) {
	full := &Target{Mode: ModeFull}
	for _, base := range []string{carBrandsQuery, carModelsQuery, productCategoriesQuery, productsQuery} {
		where, _ := deltaFilter(full)
		for _, q := range []string{base + where + " ORDER BY id", base + " WHERE COALESCE(updated_at, created_at) > ? ORDER BY id"} {
			if err := source.CheckReadOnly(q); err != nil {
				t.Errorf("guard refused %q: %v", q, err)
			}
		}
	}
	for _, q := range []string{
		whProductsQuery + " ORDER BY p.id",
		whProductsQuery + " WHERE COALESCE(p.updated_at, p.created_at) > ? ORDER BY p.id",
	} {
		if err := source.CheckReadOnly(q); err != nil {
			t.Errorf("guard refused %q: %v", q, err)
		}
	}
}

func TestReadLegacyFile(t *testing.T) {
	fsys := fstest.MapFS{
		"app/public/car-brands/a.png": {Data: []byte("public")},
		"car-brands/b.png":            {Data: []byte("root")},
		"app/public/car-brands/big":   {Data: make([]byte, 11)},
	}
	for rel, want := range map[string]string{
		"car-brands/a.png":  "public",
		"/car-brands/a.png": "public",
		"car-brands/b.png":  "root",
	} {
		got, err := readLegacyFile(fsys, rel, 10)
		if err != nil || string(got) != want {
			t.Errorf("readLegacyFile(%q) = %q, %v; want %q", rel, got, err, want)
		}
	}
	for _, rel := range []string{"car-brands/missing.png", "", "../etc/passwd"} {
		if _, err := readLegacyFile(fsys, rel, 10); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("readLegacyFile(%q) err = %v, want not exist", rel, err)
		}
	}
	if _, err := readLegacyFile(fsys, "car-brands/big", 10); !errors.Is(err, errFileTooLarge) {
		t.Errorf("oversized file err = %v", err)
	}
}

func TestWHBrand(t *testing.T) {
	for in, want := range map[string]string{
		"Olex Films": OlexBrandSlug, "": OlexBrandSlug, " GLORIAN ": "glorian", "Başka": "",
	} {
		if got := whBrand(in); got != want {
			t.Errorf("whBrand(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAvailableParts(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{`["hood", "roof"]`, `["hood","roof"]`, true},
		{``, `[]`, true},
		{`null`, `[]`, true},
		{`{"a":1}`, `[]`, false},
	}
	for _, c := range cases {
		got, ok := availableParts([]byte(c.in))
		if string(got) != c.want || ok != c.ok {
			t.Errorf("availableParts(%q) = %s, %v; want %s, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestLegacyYearAndExternalID(t *testing.T) {
	if y := legacyYear(sql.NullInt64{Int64: 2018, Valid: true}); !y.Valid || y.Int16 != 2018 {
		t.Errorf("legacyYear(2018) = %+v", y)
	}
	for _, v := range []sql.NullInt64{{}, {Int64: 0, Valid: true}, {Int64: 2200, Valid: true}} {
		if y := legacyYear(v); y.Valid {
			t.Errorf("legacyYear(%+v) = %+v, want NULL", v, y)
		}
	}
	c := counts{}
	if v := legacyExternalID(" syn-1 ", c, "brand", 1); !v.Valid || v.String != "syn-1" {
		t.Errorf("external id = %+v", v)
	}
	long := make([]byte, 65)
	for i := range long {
		long[i] = 'x'
	}
	if v := legacyExternalID(string(long), c, "brand", 2); v.Valid || c["external_id_too_long:brand:2"] != 1 {
		t.Errorf("long external id = %+v, counts %v", v, c)
	}
}

package brandctx

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNormalizeHost(t *testing.T) {
	cases := map[string]string{
		"OlexFilms.App":                    "olexfilms.app",
		"localhost:3000":                   "localhost",
		"warranty.glorianppf.com, proxy.x": "warranty.glorianppf.com",
		" olexfilms.app. ":                 "olexfilms.app",
		"[::1]:8080":                       "::1",
		"":                                 "",
	}
	for in, want := range cases {
		if got := NormalizeHost(in); got != want {
			t.Fatalf("NormalizeHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolverHostAndDefault(t *testing.T) {
	olex := Brand{ID: 1, Slug: "olex", Status: "active"}
	glorian := Brand{ID: 2, Slug: "glorian", Status: "inactive"}
	calls := 0
	r := NewResolver(func(context.Context) (Catalog, error) {
		calls++
		return Catalog{
			BySlug: map[string]Brand{"olex": olex, "glorian": glorian},
			ByHost: map[string]Brand{"olexfilms.app": olex, "warranty.glorianppf.com": glorian},
		}, nil
	}, "", time.Hour)

	b, ok, err := r.Resolve(context.Background(), "warranty.glorianppf.com:443")
	if err != nil || !ok || b.Slug != "glorian" {
		t.Fatalf("glorian host: %+v %v %v", b, ok, err)
	}
	b, ok, err = r.Resolve(context.Background(), "unknown.example")
	if err != nil || !ok || b.Slug != "olex" {
		t.Fatalf("default brand: %+v %v %v", b, ok, err)
	}
	if calls != 1 {
		t.Fatalf("catalog should be cached, loaded %d times", calls)
	}
}

func TestResolverServesStaleOnError(t *testing.T) {
	olex := Brand{ID: 1, Slug: "olex"}
	fail := false
	now := time.Now()
	r := NewResolver(func(context.Context) (Catalog, error) {
		if fail {
			return Catalog{}, errors.New("db down")
		}
		return Catalog{BySlug: map[string]Brand{"olex": olex}, ByHost: map[string]Brand{}}, nil
	}, "olex", time.Second)
	r.now = func() time.Time { return now }
	if _, _, err := r.Resolve(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	fail = true
	now = now.Add(time.Hour)
	b, ok, err := r.Resolve(context.Background(), "x")
	if err != nil || !ok || b.Slug != "olex" {
		t.Fatalf("stale snapshot expected: %+v %v %v", b, ok, err)
	}
}

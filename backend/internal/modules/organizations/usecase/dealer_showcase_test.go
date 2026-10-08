package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-250: the showcase body carries exactly the public fields.
func TestMapPublicDealerFields(t *testing.T) {
	var lat, lng pgtype.Numeric
	if err := lat.Scan("41.008200"); err != nil {
		t.Fatal(err)
	}
	if err := lng.Scan("28.978400"); err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	d := mapPublicDealer(db.GetPublicDealerBySlugRow{
		Uuid: id, Slug: "olex-kadikoy", Name: "Olex Kadıköy",
		LogoObjectKey: pgtype.Text{String: "logos/x.png", Valid: true},
		Address:       " Moda Cd. 1 ", City: "İstanbul", District: "Kadıköy",
		Latitude: lat, Longitude: lng, Phone: "+905321112233",
	})
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := "address,city,code,district,latitude,logo_url,longitude,name,whatsapp"
	if got := strings.Join(keys, ","); got != want {
		t.Fatalf("keys = %s, want %s", got, want)
	}
	if d.Address != "Moda Cd. 1" || d.Code != "olex-kadikoy" || *d.WhatsApp != "+905321112233" {
		t.Fatalf("dealer = %+v", d)
	}
	if *d.Latitude != 41.0082 || *d.Longitude != 28.9784 {
		t.Fatalf("coords = %v %v", *d.Latitude, *d.Longitude)
	}
	if d.LogoURL == nil || *d.LogoURL != "/v1/public/organizations/logo/"+id.String() {
		t.Fatalf("logo = %v", d.LogoURL)
	}
}

func TestMapPublicDealerWithoutCoordinatesOrWhatsApp(t *testing.T) {
	d := mapPublicDealer(db.GetPublicDealerBySlugRow{Slug: "x", Name: "X", Phone: "0532 111 22 33"})
	if d.Latitude != nil || d.Longitude != nil || d.WhatsApp != nil || d.LogoURL != nil {
		t.Fatalf("dealer = %+v", d)
	}
}

// TEC-251: the sitemap list carries code and updated_at only and skips
// slugs the showcase would answer 404 for.
func TestMapPublicDealerCodes(t *testing.T) {
	at := time.Date(2026, 10, 1, 9, 30, 0, 0, time.FixedZone("TRT", 3*3600))
	got := mapPublicDealerCodes([]db.ListPublicDealerCodesRow{
		{Slug: "olex-kadikoy", UpdatedAt: pgtype.Timestamptz{Time: at, Valid: true}},
		{Slug: "Bad Slug", UpdatedAt: pgtype.Timestamptz{Time: at, Valid: true}},
	})
	if len(got) != 1 || got[0].Code != "olex-kadikoy" || !got[0].UpdatedAt.Equal(at) || got[0].UpdatedAt.Location() != time.UTC {
		t.Fatalf("codes = %+v", got)
	}
	b, err := json.Marshal(got[0])
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"code":"olex-kadikoy","updated_at":"2026-10-01T06:30:00Z"}`; string(b) != want {
		t.Fatalf("json = %s, want %s", b, want)
	}
	if empty := mapPublicDealerCodes(nil); empty == nil || len(empty) != 0 {
		t.Fatalf("empty = %#v, want []", empty)
	}
}

// A malformed code is 404 without touching the database.
func TestPublicDealerByCodeMalformed(t *testing.T) {
	s := &Service{}
	for _, code := range []string{"", "  ", "-x", "a/b", "a b", "ö", strings.Repeat("a", 101)} {
		if _, err := s.PublicDealerByCode(context.Background(), 1, code, ""); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%q: err = %v, want ErrNotFound", code, err)
		}
	}
}

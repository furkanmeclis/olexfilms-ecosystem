package usecase

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestPortalOwns(t *testing.T) {
	svc := db.Service{CustomerUserID: 7}
	ws := []db.Warranty{{HolderUserID: 9}}
	cases := []struct {
		user int64
		want bool
	}{{7, true}, {9, true}, {8, false}, {0, false}}
	for _, c := range cases {
		if got := portalOwns(svc, c.user, ws); got != c.want {
			t.Fatalf("portalOwns(user %d) = %v, want %v", c.user, got, c.want)
		}
	}
}

func samplePortal(phone string) PortalServiceView {
	now := time.Now().UTC()
	svc := db.Service{
		ID: 1, Uuid: uuid.New(), ServiceNo: "DS-239", Status: StatusCompleted, CustomerUserID: 7,
		Plate:          pgtype.Text{String: "34ABC239", Valid: true},
		Km:             pgtype.Int4{Int32: 12000, Valid: true},
		Notes:          pgtype.Text{String: "staff note", Valid: true},
		HasMeasurement: true, MeasurementResultID: pgtype.Int8{Int64: 5, Valid: true},
		CreatedAt:   pgtype.Timestamptz{Time: now, Valid: true},
		CompletedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}
	refs := db.GetServiceRefsRow{CustomerName: "Ayşe", CustomerPhone: pgtype.Text{String: "+905551234567", Valid: true},
		CarBrandName: "BMW", CarModelName: "320i"}
	org := db.Organization{Uuid: uuid.New(), Name: "Dealer", City: "İstanbul", Address: "Cad. 1", Phone: phone}
	items := []portalItem{
		{ID: 10, UUID: uuid.New(), Product: "Film", Category: "PPF", Parts: []string{"body_tavan", "body_kaput"}},
		{ID: 11, UUID: uuid.New(), Product: "Roll", Category: "Cam", Parts: []string{"body_kaput"}},
		{ID: 12, UUID: uuid.New(), Product: "Bare"},
	}
	ws := []db.Warranty{
		{ID: 1, Uuid: uuid.New(), PublicCode: "MINE123456789", ServiceItemID: 10, HolderUserID: 7, Status: "active", ItemKind: "full"},
		{ID: 2, Uuid: uuid.New(), PublicCode: "OTHER12345678", ServiceItemID: 11, HolderUserID: 8, Status: "active", ItemKind: "partial"},
	}
	return portalView(svc, refs, org, items, ws, 7, i18n.LocaleTR)
}

func TestPortalViewProjection(t *testing.T) {
	v := samplePortal("+902165550000")
	if strings.Join(v.AppliedParts, ",") != "body_kaput,body_tavan" {
		t.Fatalf("applied parts = %v", v.AppliedParts)
	}
	if len(v.Products) != 3 || v.Products[0].Category != "PPF" || v.Products[2].AppliedParts == nil {
		t.Fatalf("products = %+v", v.Products)
	}
	if v.Dealer.WhatsApp == nil || *v.Dealer.WhatsApp != "+902165550000" || v.Dealer.Address != "Cad. 1" {
		t.Fatalf("dealer = %+v", v.Dealer)
	}
	if len(v.Warranties) != 1 || v.Warranties[0].PublicCode != "MINE123456789" || v.Warranties[0].ProductName != "Film" {
		t.Fatalf("warranties must list only the held ones: %+v", v.Warranties)
	}
	if v.Plate == nil || *v.Plate != "34ABC239" {
		t.Fatalf("plate = %v", v.Plate)
	}
	if samplePortal("").Dealer.WhatsApp != nil {
		t.Fatal("empty dealer phone must give a null whatsapp")
	}
}

// TEC-239: the portal projection carries no measurement field, no price,
// no stock detail, no staff note and no customer contact data.
func TestPortalViewHidesMeasurementAndPrices(t *testing.T) {
	b, err := json.Marshal(samplePortal("+902165550000"))
	if err != nil {
		t.Fatal(err)
	}
	var raw any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, val := range x {
				keys[k] = true
				walk(val)
			}
		case []any:
			for _, val := range x {
				walk(val)
			}
		}
	}
	walk(raw)
	for _, k := range []string{"has_measurement", "measurement_result_id", "measurement", "price", "purchase_price",
		"sale_price", "recommended_sale_price", "barcode", "meters", "quantity", "notes", "km", "customer", "phone"} {
		if keys[k] {
			t.Fatalf("portal view carries %q: %s", k, b)
		}
	}
	for _, leak := range []string{"staff note", "+905551234567", "Ayşe", "12000"} {
		if strings.Contains(string(b), leak) {
			t.Fatalf("portal view leaks %q: %s", leak, b)
		}
	}
}

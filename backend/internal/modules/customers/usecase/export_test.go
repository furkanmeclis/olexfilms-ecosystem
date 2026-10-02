package usecase

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/google/uuid"
)

func sampleExport() DataExport {
	s := func(v string) *string { return &v }
	return DataExport{
		Version: DataExportVersion, ExportedAt: time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC),
		Profile: ExportProfile{
			UUID: uuid.New(), Name: "Ayşe <b>", Surname: "Kaya", Email: s("a@example.test"), Phone: s("+905551112233"),
			Status: StatusActive, Type: TypeIndividual, NationalIDLast4: s("8901"),
			Address: json.RawMessage(`{"city":"İstanbul","line1":"Gizli Sokak 7"}`), NotificationPrefs: json.RawMessage(`{}`),
		},
		Vehicles: []ExportVehicle{{UUID: uuid.New(), Plate: s("34 TEC 161"), PlateCountry: s("TR"), VIN: s("WVWZZZ1JZXW000161")}},
		Services: []ExportService{{UUID: uuid.New(), ServiceNo: "DS00000001", Status: "completed", Organization: "Bayi", CarBrand: "VW", CarModel: "Golf"}},
		Warranties: []ExportWarranty{{
			UUID: uuid.New(), PublicCode: "abcdefghijkl", Status: "active", Product: "PPF",
			StartAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), EndAt: time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC),
		}},
	}
}

func TestDataExportJSONDocument(t *testing.T) {
	a := NewDataExportAdapter(nil)
	ds := DataExportDataset(a.Resource(), sampleExport(), i18n.LocaleTR)
	b, err := a.DocumentJSON(ds, "tr")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"version", "exported_at", "profile", "vehicles", "services", "warranties"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("json document misses %q", k)
		}
	}
	if len(ds.Rows) == 0 || ds.Rows[0]["section"] != "Profil" {
		t.Fatalf("flat rows = %v", ds.Rows)
	}
	var _ ioengine.JSONDocumentRenderer = a
}

func TestDataExportHTMLEscapesAndLocalizes(t *testing.T) {
	a := NewPortalDataExportAdapter(nil)
	ds := DataExportDataset(a.Resource(), sampleExport(), i18n.LocaleAR)
	h, err := a.DocumentHTML(ds, "ar", nil, "تصدير")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(h, "Ayşe <b>") || !strings.Contains(h, "Ayşe &lt;b&gt;") {
		t.Fatal("names must be escaped")
	}
	for _, want := range []string{`dir="rtl"`, "34 TEC 161 (TR)", "•••• 8901", "DS00000001", "abcdefghijkl", "Gizli Sokak 7"} {
		if !strings.Contains(h, want) {
			t.Fatalf("html misses %q", want)
		}
	}
}

func TestDataExportMasksAnonymizedProfileInSections(t *testing.T) {
	doc := sampleExport()
	doc.Profile.Status = StatusAnonymized
	doc.Profile.Anonymized = true
	doc.Profile.Name, doc.Profile.Surname = i18n.Translate(i18n.LocaleEN, AnonymizedNameKey), ""
	doc.Profile.Email, doc.Profile.Phone, doc.Profile.NationalIDLast4 = nil, nil, nil
	doc.Profile.Address = json.RawMessage("{}")
	secs := dataSections(doc, i18n.LocaleEN)
	for _, kv := range secs[0].records[0] {
		if strings.Contains(kv[1], "@") || strings.Contains(kv[1], "+90") || strings.Contains(kv[1], "8901") {
			t.Fatalf("anonymized profile leaks %q=%q", kv[0], kv[1])
		}
	}
	if len(secs[1].records) != 1 || secs[1].records[0][0][1] != "34 TEC 161 (TR)" {
		t.Fatal("vehicle plate must stay after anonymization")
	}
}

package ioengine

import (
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
)

func TestExportTitle(t *testing.T) {
	if got := ExportTitle("tr", "platform.users"); got != "Kullanıcılar dışa aktarma" {
		t.Fatalf("tr title = %q", got)
	}
	if got := ExportTitle("en", "platform.users"); got != "Users export" {
		t.Fatalf("en title = %q", got)
	}
}

func TestFormatDatetimeLocale(t *testing.T) {
	ts := time.Date(2026, 8, 23, 14, 53, 0, 0, time.UTC)
	if got := formatDatetime(ts, i18n.LocaleTR); got != "23.08.2026 14:53 UTC" {
		t.Fatalf("tr datetime = %q", got)
	}
	if got := formatDatetime(ts, i18n.LocaleEN); got != "2026-08-23 14:53 UTC" {
		t.Fatalf("en datetime = %q", got)
	}
}

func TestFormatCellEnumAndDatetime(t *testing.T) {
	col := Column{Key: "status", LabelKey: "users.status", Type: ColumnTypeEnum}
	if got := formatCell("active", col, i18n.LocaleTR); got != "Aktif" {
		t.Fatalf("enum = %q", got)
	}
	dtCol := Column{Key: "created_at", LabelKey: "users.created_at", Type: ColumnTypeDatetime}
	ts := time.Date(2026, 8, 23, 17, 47, 18, 0, time.UTC)
	if got := formatCell(ts, dtCol, i18n.LocaleTR); got != "23.08.2026 17:47 UTC" {
		t.Fatalf("datetime cell = %q", got)
	}
}

func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("Oluşturulma", 5); got != "Oluş…" {
		t.Fatalf("truncate = %q", got)
	}
}

func TestEncodePDFTurkish(t *testing.T) {
	ds := Dataset{
		Resource: "platform.users",
		Columns: []Column{
			{Key: "name", LabelKey: "users.name", Type: ColumnTypeString},
			{Key: "created_at", LabelKey: "users.created_at", Type: ColumnTypeDatetime},
		},
		Rows: []map[string]any{
			{"name": "Yıldız", "created_at": time.Date(2026, 8, 23, 17, 47, 18, 0, time.UTC)},
		},
	}
	out, err := EncodePDF(ds, "tr", nil, ExportTitle("tr", "platform.users"))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) < 1000 {
		t.Fatalf("pdf too small: %d bytes", len(out))
	}
}

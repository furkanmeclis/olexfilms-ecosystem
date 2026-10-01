package ioengine

import (
	"strings"
	"testing"
)

// TEC-138: CSV headers and enum cells follow the export locale (ar, zh-CN).
func TestEncodeCSVTranslatedHeaders(t *testing.T) {
	ds := Dataset{
		Resource: "platform.users",
		Columns: []Column{
			{Key: "email", LabelKey: "users.email", Type: ColumnTypeString},
			{Key: "status", LabelKey: "users.status", Type: ColumnTypeEnum},
		},
		Rows: []map[string]any{{"email": "a@example.com", "status": "active"}},
	}
	cases := map[string][]string{
		"ar":    {"البريد الإلكتروني,الحالة", "a@example.com,نشط"},
		"zh-CN": {"电子邮箱,状态", "a@example.com,启用"},
		"bg":    {"Имейл,Статус", "a@example.com,Активен"},
	}
	for loc, want := range cases {
		out, err := EncodeCSV(ds, loc)
		if err != nil {
			t.Fatalf("%s: %v", loc, err)
		}
		lines := strings.Split(strings.TrimSpace(strings.TrimPrefix(string(out), "\xEF\xBB\xBF")), "\n")
		if len(lines) != 2 || lines[0] != want[0] || lines[1] != want[1] {
			t.Errorf("%s csv = %q, want %q", loc, lines, want)
		}
	}
}

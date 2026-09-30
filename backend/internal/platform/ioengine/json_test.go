package ioengine

import (
	"testing"
	"time"
)

func TestEncodeJSON(t *testing.T) {
	ds := Dataset{
		Resource: "platform.users",
		Columns: []Column{
			{Key: "email", LabelKey: "users.email", Type: ColumnTypeString},
			{Key: "created_at", LabelKey: "users.created_at", Type: ColumnTypeDatetime},
		},
		Rows: []map[string]any{
			{
				"email":      "user@example.com",
				"created_at": time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC),
			},
		},
	}
	out, err := EncodeJSON(ds, "tr")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) < 50 {
		t.Fatalf("json too short: %s", out)
	}
}

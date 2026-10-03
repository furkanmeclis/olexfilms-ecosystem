package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator"
)

// Bad input is refused before any database connection is opened.
func TestRunRefusesBadArgsBeforeConnecting(t *testing.T) {
	ctx := context.Background()
	for _, args := range [][]string{
		nil,
		{"--since=yesterday"},
		{"--since=2026-11-01"},
		{"--since=2026-11-01T03:00:00+03:00", "--format=xlsx"},
		{"--since=2026-11-01T03:00:00+03:00", "extra"},
	} {
		var out, errOut bytes.Buffer
		if err := run(ctx, args, &out, &errOut); err == nil {
			t.Errorf("run(%v) succeeded", args)
		}
		if out.Len() != 0 {
			t.Errorf("run(%v) wrote to stdout: %q", args, out.String())
		}
	}
}

func TestParseArgs(t *testing.T) {
	o, err := parseArgs([]string{"--since=2026-11-01T03:00:00+03:00"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if o.format != "csv" || !o.since.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("options = %+v", o)
	}
	if o, err = parseArgs([]string{"--since=2026-11-01T00:00:00Z", "--format=json"}, io.Discard); err != nil || o.format != "json" {
		t.Fatalf("json options = %+v, %v", o, err)
	}
}

// The CSV header row is fixed, also for an empty report; JSON of an empty
// report is [].
func TestWriteFormats(t *testing.T) {
	var buf bytes.Buffer
	if err := write(&buf, "csv", nil); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "table,uuid,organization,created_at,summary\n" {
		t.Fatalf("empty csv = %q", got)
	}

	id := uuid.MustParse("0b6f1a52-6a43-4f3e-9d0e-2b1e6f0f5a01")
	rows := []migrator.DeltaRow{{
		Table: "services", UUID: id, Organization: "Bayi, Kadıköy",
		CreatedAt: time.Date(2026, 11, 1, 1, 2, 3, 0, time.UTC), Summary: "DS00000001 draft TR 34ABC123",
	}}
	buf.Reset()
	if err := write(&buf, "csv", rows); err != nil {
		t.Fatal(err)
	}
	want := "table,uuid,organization,created_at,summary\n" +
		"services," + id.String() + ",\"Bayi, Kadıköy\",2026-11-01T01:02:03Z,DS00000001 draft TR 34ABC123\n"
	if buf.String() != want {
		t.Fatalf("csv = %q, want %q", buf.String(), want)
	}

	buf.Reset()
	if err := write(&buf, "json", nil); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(buf.String()) != "[]" {
		t.Fatalf("empty json = %q", buf.String())
	}
	buf.Reset()
	if err := write(&buf, "json", rows); err != nil {
		t.Fatal(err)
	}
	var back []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &back); err != nil {
		t.Fatal(err)
	}
	if len(back) != 1 || back[0]["table"] != "services" || back[0]["uuid"] != id.String() ||
		back[0]["organization"] != "Bayi, Kadıköy" || back[0]["created_at"] != "2026-11-01T01:02:03Z" {
		t.Fatalf("json = %v", back)
	}
}

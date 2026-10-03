package migrator

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDeltaSince(t *testing.T) {
	wm := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		overlap time.Duration
		want    time.Time
	}{
		{0, wm.Add(-DefaultDeltaOverlap)},
		{time.Hour, wm.Add(-time.Hour)},
		{-1, wm},
	} {
		if got := DeltaSince(wm, c.overlap); !got.Equal(c.want) {
			t.Errorf("DeltaSince(%v) = %v, want %v", c.overlap, got, c.want)
		}
	}
	if got := DeltaSince(time.Time{}, time.Hour); !got.IsZero() {
		t.Errorf("DeltaSince(zero) = %v, want zero (full read)", got)
	}
}

func TestSkipReasons(t *testing.T) {
	keys := []string{
		"warranty_skipped_service_not_completed:4",
		"warranty_skipped_service_not_completed", // the total, no id
		"skipped_no_contact:user:9",
		"lines_skipped_product_unmapped:wh:0000000c-0000-4000-8000-000000000001",
		"warranties_created",
		"unmapped:7",
	}
	got := skipReasons(keys, []string{"warranty_skipped_", "skipped_no_contact:", "lines_skipped_"})
	want := map[string]string{
		"4":                                    "warranty_skipped_service_not_completed",
		"9":                                    "skipped_no_contact:user",
		"0000000c-0000-4000-8000-000000000001": "lines_skipped_product_unmapped:wh",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("skipReasons = %v, want %v", got, want)
	}
}

func TestLegacyIDAndSort(t *testing.T) {
	u := uuid.MustParse("00000008-0000-4000-8000-000000000001")
	for _, c := range []struct {
		in   any
		want string
	}{
		{int64(12), "12"}, {int32(3), "3"}, {"abc  ", "abc"}, {[]byte("42"), "42"}, {[16]byte(u), u.String()},
	} {
		got, err := legacyID(c.in)
		if err != nil || got != c.want {
			t.Errorf("legacyID(%v) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
	if _, err := legacyID(nil); err == nil {
		t.Error("legacyID(nil) succeeded")
	}
	ids := []string{"10", "9", "100", "2"}
	sortIDs(ids)
	if strings.Join(ids, ",") != "2,9,10,100" {
		t.Errorf("sortIDs = %v", ids)
	}
}

func TestReportErrAndText(t *testing.T) {
	target := uuid.New()
	rep := &Report{Profile: "olex", GeneratedAt: time.Now(),
		Tables: []TableReport{{Source: SourceHub, Table: "warranties", TargetTable: "warranties", SourceRows: 4,
			Migrated: 3, TargetRows: 2, Merged: 2, Skipped: 1}},
		Expected: []ReportDiff{
			{Source: SourceHub, Table: "warranties", ID: "1", Class: ClassMerged, Reason: "duplicate_warranty", Target: &target},
			{Source: SourceHub, Table: "warranties", ID: "2", Class: ClassMerged, Reason: "duplicate_warranty", Target: &target},
			{Source: SourceHub, Table: "warranties", ID: "4", Class: ClassSkipped, Reason: "warranty_skipped_service_not_completed"},
		},
	}
	if err := rep.Err(); err != nil {
		t.Fatalf("Err = %v", err)
	}
	var buf bytes.Buffer
	if err := WriteReportText(&buf, rep); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"hub.warranties", "duplicate_warranty", "1,2", "Mismatches (0)"} {
		if !strings.Contains(buf.String(), s) {
			t.Errorf("text report misses %q:\n%s", s, buf.String())
		}
	}
	rep.Mismatches = append(rep.Mismatches, ReportDiff{Source: SourceHub, Table: "short_urls", ID: "1", Class: MismatchTargetMissing})
	if err := rep.Err(); !errors.Is(err, ErrReportMismatch) {
		t.Errorf("Err with a mismatch = %v", err)
	}
}

// Every report table names a step of the olex profile, and the profile
// lists its steps in run order (delta keeps it).
func TestOlexReportTablesSteps(t *testing.T) {
	steps := map[string]bool{}
	for _, s := range olexSteps() {
		steps[s.Name()] = true
	}
	seen := map[string]bool{}
	for _, tb := range OlexReportTables() {
		if !steps[tb.Step] {
			t.Errorf("%s.%s: step %q is not an olex step", tb.Source, tb.Table, tb.Step)
		}
		k := tb.Source + "." + tb.Table
		if seen[k] {
			t.Errorf("%s listed twice", k)
		}
		seen[k] = true
	}
}

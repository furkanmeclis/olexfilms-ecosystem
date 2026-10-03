package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/migrator"
)

// The command refuses the disabled Glorian profile and bad input before it
// opens any database connection.
func TestRunCommandRefusesBeforeConnecting(t *testing.T) {
	ctx := context.Background()
	var out bytes.Buffer
	if err := run(ctx, []string{"run", "--profile=glorian"}, &out); !errors.Is(err, migrator.ErrProfileDisabled) {
		t.Fatalf("run --profile=glorian = %v, want ErrProfileDisabled", err)
	}
	if err := run(ctx, []string{"run", "--profile=olex", "--mode=partial"}, &out); err == nil {
		t.Fatal("run --mode=partial succeeded")
	}
	if err := run(ctx, nil, &out); err == nil {
		t.Fatal("no command succeeded")
	}
	if err := run(ctx, []string{"migrate-all"}, &out); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("unknown command = %v", err)
	}
	if err := run(ctx, []string{"runs", "--limit=0"}, &out); err == nil {
		t.Fatal("runs --limit=0 succeeded")
	}
}

func TestOpenLegacyNeedsDSN(t *testing.T) {
	t.Setenv("LEGACY_HUB_DSN", "")
	if _, err := openLegacy(context.Background(), migrator.SourceHub); err == nil || !strings.Contains(err.Error(), "LEGACY_HUB_DSN") {
		t.Fatalf("openLegacy without DSN = %v", err)
	}
	if _, err := openLegacy(context.Background(), "glorian"); err == nil {
		t.Fatal("openLegacy(unknown) succeeded")
	}
}

// TEC-264: report refuses a disabled profile before connecting, and a report
// with a mismatch is an error (the command exits non-zero) in both formats.
func TestReportCommand(t *testing.T) {
	ctx := context.Background()
	var out bytes.Buffer
	if err := run(ctx, []string{"report", "--profile=glorian"}, &out); !errors.Is(err, migrator.ErrProfileDisabled) {
		t.Fatalf("report --profile=glorian = %v, want ErrProfileDisabled", err)
	}
	if err := run(ctx, []string{"run", "--overlap=-1m"}, &out); err == nil {
		t.Fatal("run --overlap=-1m succeeded")
	}

	clean := &migrator.Report{Profile: "olex", Tables: []migrator.TableReport{{Source: "hub", Table: "dealers", SourceRows: 1, Migrated: 1}}}
	bad := &migrator.Report{Profile: "olex", Mismatches: []migrator.ReportDiff{
		{Source: "hub", Table: "short_urls", ID: "1", Class: migrator.MismatchTargetMissing},
	}}
	for _, asJSON := range []bool{false, true} {
		out.Reset()
		if err := writeReport(&out, clean, asJSON); err != nil {
			t.Errorf("clean report (json=%v) = %v", asJSON, err)
		}
		out.Reset()
		if err := writeReport(&out, bad, asJSON); !errors.Is(err, migrator.ErrReportMismatch) {
			t.Errorf("mismatching report (json=%v) = %v, want ErrReportMismatch", asJSON, err)
		}
		if !strings.Contains(out.String(), "target_missing") {
			t.Errorf("output (json=%v) does not list the mismatch:\n%s", asJSON, out.String())
		}
	}
}

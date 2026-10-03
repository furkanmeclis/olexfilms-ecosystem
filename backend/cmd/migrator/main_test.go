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
	if err := run(ctx, []string{"report", "--limit=0"}, &out); err == nil {
		t.Fatal("report --limit=0 succeeded")
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

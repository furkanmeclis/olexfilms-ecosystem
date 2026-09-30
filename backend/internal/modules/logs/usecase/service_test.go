package usecase

import (
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/logs/model"
)

func TestRuleIsDue(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)

	t.Run("never_run", func(t *testing.T) {
		t.Parallel()
		if !RuleIsDue(nil, 1440, now) {
			t.Fatal("expected due when last_run_at is nil")
		}
	})

	t.Run("interval_elapsed", func(t *testing.T) {
		t.Parallel()
		last := now.Add(-25 * time.Hour)
		if !RuleIsDue(&last, 1440, now) {
			t.Fatal("expected due after daily interval")
		}
	})

	t.Run("interval_pending", func(t *testing.T) {
		t.Parallel()
		last := now.Add(-30 * time.Minute)
		if RuleIsDue(&last, 60, now) {
			t.Fatal("expected not due before hourly interval")
		}
	})

	t.Run("invalid_interval", func(t *testing.T) {
		t.Parallel()
		if RuleIsDue(nil, 0, now) {
			t.Fatal("interval 0 must not be due")
		}
	})
}

func TestValidateRuleValues(t *testing.T) {
	t.Parallel()
	ok := model.PurgeRule{
		Name:            "Cleanup debug",
		Levels:          []string{"debug"},
		OlderThanHours:  24,
		IntervalMinutes: 60,
	}
	if err := validateRuleValues(ok); err != nil {
		t.Fatalf("valid rule rejected: %v", err)
	}

	bad := model.PurgeRule{
		Name:            "",
		Levels:          []string{"fatal"},
		OlderThanHours:  0,
		IntervalMinutes: 7,
	}
	if err := validateRuleValues(bad); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestNormalizeLevels(t *testing.T) {
	t.Parallel()
	got := normalizeLevels([]string{" Warn ", "warn", "error", ""})
	if len(got) != 2 || got[0] != "warn" || got[1] != "error" {
		t.Fatalf("unexpected levels: %#v", got)
	}
}

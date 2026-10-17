package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-769: organization-level warehouse settings. EODCutoff is the
// end-of-day close ("18:00") in the organization's time zone; nil keeps the
// calendar day (reports cover 00:00-24:00 and are written after midnight).

// WarehouseSettings is the API view of the settings.
type WarehouseSettings struct {
	EODCutoff *string `json:"eod_cutoff"`
	Timezone  string  `json:"timezone"`
}

// WarehouseSettingsInput updates the settings; a nil or empty EODCutoff
// (or "00:00", which is midnight) clears the cutoff.
type WarehouseSettingsInput struct {
	EODCutoff *string
}

// ParseCutoff parses "HH:MM" into minutes after midnight; "" and "00:00"
// mean no cutoff (nil).
func ParseCutoff(raw string) (*int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse("15:04", raw)
	if err != nil || len(raw) != 5 {
		return nil, invalid("eod_cutoff", "must be a time HH:MM (00:00-23:59)")
	}
	m := t.Hour()*60 + t.Minute()
	if m == 0 {
		return nil, nil
	}
	return &m, nil
}

func formatCutoff(m *int) *string {
	if m == nil {
		return nil
	}
	s := fmt.Sprintf("%02d:%02d", *m/60, *m%60)
	return &s
}

// Settings returns the warehouse settings of the active organization.
func (s *EOD) Settings(ctx context.Context, c Caller) (WarehouseSettings, error) {
	orgID, err := guard(c)
	if err != nil {
		return WarehouseSettings{}, err
	}
	return s.settingsView(ctx, orgID)
}

// UpdateSettings writes the warehouse settings of the active organization.
func (s *EOD) UpdateSettings(ctx context.Context, c EODCaller, in WarehouseSettingsInput) (WarehouseSettings, error) {
	orgID, err := guard(c.Caller)
	if err != nil {
		return WarehouseSettings{}, err
	}
	var cutoff *int
	if in.EODCutoff != nil {
		if cutoff, err = ParseCutoff(*in.EODCutoff); err != nil {
			return WarehouseSettings{}, err
		}
	}
	arg := db.UpsertWarehouseSettingsParams{OrganizationID: orgID}
	if cutoff != nil {
		arg.EodCutoffMinutes = pgtype.Int2{Int16: int16(*cutoff), Valid: true}
	}
	if _, err := s.q.UpsertWarehouseSettings(ctx, arg); err != nil {
		return WarehouseSettings{}, err
	}
	return s.settingsView(ctx, orgID)
}

func (s *EOD) settingsView(ctx context.Context, orgID int64) (WarehouseSettings, error) {
	org, err := s.q.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return WarehouseSettings{}, fmt.Errorf("warehouse settings: organization: %w", err)
	}
	cutoff, err := s.cutoff(ctx, orgID)
	if err != nil {
		return WarehouseSettings{}, err
	}
	_, tz := zoneOf(org.Timezone)
	return WarehouseSettings{EODCutoff: formatCutoff(cutoff), Timezone: tz}, nil
}

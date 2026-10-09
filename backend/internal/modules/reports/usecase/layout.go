package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	// LayoutVersion is the only layout schema version.
	LayoutVersion = 1
	// MaxWidgets caps a layout.
	MaxWidgets = 30
)

// defaultWidgetID is the id of the default overview widget (legacy layout.md).
var defaultWidgetID = uuid.MustParse("00000000-0000-4000-8000-000000000001")

// Widget is one pinned report.
type Widget struct {
	ID          uuid.UUID `json:"id"`
	Report      string    `json:"report"`
	SortOrder   int       `json:"sort_order"`
	Period      *string   `json:"period"`
	Granularity *string   `json:"granularity"`
}

// Layout is the user's widget list in the active organization.
type Layout struct {
	Version   int        `json:"version"`
	UpdatedAt *time.Time `json:"updated_at"`
	Widgets   []Widget   `json:"widgets"`
}

// WidgetInput is one widget of a PUT.
type WidgetInput struct {
	ID          string  `json:"id"`
	Report      string  `json:"report"`
	Period      *string `json:"period"`
	Granularity *string `json:"granularity"`
}

// LayoutInput is the full replacement layout.
type LayoutInput struct {
	Version *int          `json:"version"`
	Widgets []WidgetInput `json:"widgets"`
}

// GetLayout returns the stored layout (or the default one: overview only),
// without the widgets whose report the caller can no longer open.
func (s *Service) GetLayout(ctx context.Context, c Caller) (Layout, error) {
	row, err := s.q.GetReportLayout(ctx, db.GetReportLayoutParams{UserID: c.Principal.UserInternal, OrganizationID: c.Org.InternalID})
	var stored []Widget
	out := Layout{Version: LayoutVersion}
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		stored = []Widget{{ID: defaultWidgetID, Report: ReportOverview}}
	case err != nil:
		return Layout{}, fmt.Errorf("reports: get layout: %w", err)
	default:
		if err := json.Unmarshal(row.Widgets, &stored); err != nil {
			return Layout{}, fmt.Errorf("reports: decode layout: %w", err)
		}
		out.Version = int(row.Version)
		if row.UpdatedAt.Valid {
			t := row.UpdatedAt.Time.UTC()
			out.UpdatedAt = &t
		}
	}
	out.Widgets = make([]Widget, 0, len(stored))
	open := map[string]bool{}
	for _, w := range stored {
		ok, seen := open[w.Report]
		if !seen {
			d, known := DefinitionByKey(w.Report)
			if known {
				if ok, err = s.accessible(ctx, c, d); err != nil {
					return Layout{}, err
				}
			}
			open[w.Report] = ok
		}
		if ok {
			w.SortOrder = len(out.Widgets)
			out.Widgets = append(out.Widgets, w)
		}
	}
	return out, nil
}

// PutLayout replaces the layout atomically: every widget is validated
// first and one invalid widget (unknown or unavailable report, duplicate
// id or report, bad period / granularity) rejects the whole layout; the
// stored layout stays unchanged.
func (s *Service) PutLayout(ctx context.Context, c Caller, in LayoutInput) (Layout, error) {
	if in.Version != nil && *in.Version != LayoutVersion {
		return Layout{}, invalid("version", "invalid", "version must be 1")
	}
	if in.Widgets == nil {
		return Layout{}, invalid("widgets", "required", "widgets is required")
	}
	if len(in.Widgets) > MaxWidgets {
		return Layout{}, invalid("widgets", "too_many", "at most 30 widgets")
	}
	widgets := make([]Widget, 0, len(in.Widgets))
	ids := map[uuid.UUID]bool{}
	reports := map[string]bool{}
	for i, w := range in.Widgets {
		field := "widgets[" + strconv.Itoa(i) + "]"
		id, err := uuid.Parse(w.ID)
		if err != nil {
			return Layout{}, invalid(field+".id", "invalid", "id must be a UUID")
		}
		if ids[id] {
			return Layout{}, invalid(field+".id", "duplicate", "widget ids must be unique")
		}
		ids[id] = true
		d, ok := DefinitionByKey(w.Report)
		if !ok {
			return Layout{}, invalid(field+".report", "unknown", "unknown report")
		}
		if reports[d.Key] {
			return Layout{}, invalid(field+".report", "duplicate", "a report can be pinned once")
		}
		reports[d.Key] = true
		open, err := s.accessible(ctx, c, d)
		if err != nil {
			return Layout{}, err
		}
		if !open {
			return Layout{}, invalid(field+".report", "unavailable", "this report is not available to you")
		}
		if w.Period != nil {
			if !d.Periodic {
				return Layout{}, invalid(field+".period", "unsupported", "this report has no period")
			}
			if !contains(Periods, *w.Period) {
				return Layout{}, invalid(field+".period", "invalid", "period must be one of 7d, 30d, 90d, 12m")
			}
		}
		if w.Granularity != nil {
			if !d.Granular {
				return Layout{}, invalid(field+".granularity", "unsupported", "this report has no granularity")
			}
			if !contains(Granularities, *w.Granularity) {
				return Layout{}, invalid(field+".granularity", "invalid", "granularity must be day, week or month")
			}
		}
		widgets = append(widgets, Widget{ID: id, Report: d.Key, SortOrder: i, Period: w.Period, Granularity: w.Granularity})
	}
	raw, err := json.Marshal(widgets)
	if err != nil {
		return Layout{}, err
	}
	row, err := s.q.UpsertReportLayout(ctx, db.UpsertReportLayoutParams{
		UserID: c.Principal.UserInternal, OrganizationID: c.Org.InternalID, BrandID: c.Org.BrandID,
		Version: LayoutVersion, Widgets: raw,
	})
	if err != nil {
		return Layout{}, fmt.Errorf("reports: save layout: %w", err)
	}
	t := row.UpdatedAt.Time.UTC()
	return Layout{Version: int(row.Version), UpdatedAt: &t, Widgets: widgets}, nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

package usecase

// TEC-1058 (RV-5.6): monthly plotter-cut shipments. The order lines marked
// "sent plotter cut" are counted in the month their order shipped (orders
// cancelled after shipping are left out), per product: line count, pieces
// and meters. The series carry the monthly totals (every month of the
// range, missing = 0), the rows the month x product breakdown.

import (
	"context"
	"fmt"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
)

const (
	unitPieces = "pieces"
	unitMeters = "meters"
)

// months lists the first day of every month the range touches.
func (q query) months() []string {
	var out []string
	m := time.Date(q.fromDate.Year(), q.fromDate.Month(), 1, 0, 0, 0, 0, time.UTC)
	for !m.After(q.toDate) {
		out = append(out, m.Format(dateLayout))
		m = m.AddDate(0, 1, 0)
	}
	return out
}

func (s *Service) plotterCut(ctx context.Context, q query, a args, env *Envelope) error {
	rows, err := s.q.ReportOrderPlotterCut(ctx, db.ReportOrderPlotterCutParams{
		Tz: q.tz.String(), BrandID: a.brandID, OrgIds: a.orgIDs, RangeFrom: ts(q.from), RangeTo: ts(q.to),
	})
	if err != nil {
		return fmt.Errorf("reports: plotter cut: %w", err)
	}
	lines, pieces, meters := map[string]float64{}, map[string]float64{}, map[string]float64{}
	env.Columns = q.columns([][2]string{
		{"month", "string"}, {"product", "string"}, {"sku", "string"},
		{"line_count", "integer"}, {"quantity", "integer"}, {"meters", "number"},
	})
	for _, r := range rows {
		if !r.Month.Valid {
			continue
		}
		month := r.Month.Time.Format(dateLayout)
		lines[month] += float64(r.LineCount)
		pieces[month] += float64(r.Quantity)
		meters[month] += r.Meters
		env.Rows = append(env.Rows, map[string]any{
			"month": r.Month.Time.Format("2006-01"), "product_uuid": r.ProductUuid, "product": r.ProductName,
			"sku": r.Sku, "unit_type": r.UnitType, "line_count": r.LineCount, "quantity": r.Quantity, "meters": r.Meters,
		})
	}
	series := func(key, unit string, values map[string]float64) Series {
		sr := q.series(key, unit)
		for _, m := range q.months() {
			sr.Points = append(sr.Points, Point{Key: m, Label: m[:7], Value: values[m]})
		}
		return sr
	}
	env.Series = []Series{
		series("plotter_lines", unitCount, lines),
		series("plotter_quantity", unitPieces, pieces),
		series("plotter_meters", unitMeters, meters),
	}
	return nil
}

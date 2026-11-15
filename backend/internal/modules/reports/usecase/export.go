package usecase

// TEC-1210: GET /v1/reports/{report}/export — the same report (same query,
// permission and scope) as an XLSX or CSV file. A table report exports its
// columns and rows; a time series report one row per bucket with a column
// per series; the others (summaries, distributions, rankings) one row per
// KPI and series point. Headers are in the request locale; number, money,
// percent and duration cells are numbers; times are UTC (as the other
// exports).

import (
	"context"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
)

// ExportFormats are the formats of a report export.
var ExportFormats = []string{string(ioengine.ExportXLSX), string(ioengine.ExportCSV)}

// ExportFile is an encoded report.
type ExportFile struct {
	Filename    string
	ContentType string
	Body        []byte
}

// Export builds the report and encodes it.
func (s *Service) Export(ctx context.Context, c Caller, key, format string, in Input) (ExportFile, error) {
	f := ioengine.ExportFormat(strings.ToLower(strings.TrimSpace(format)))
	if f == "" {
		f = ioengine.ExportXLSX
	}
	if f != ioengine.ExportXLSX && f != ioengine.ExportCSV {
		return ExportFile{}, invalid("format", "invalid", "format must be xlsx or csv")
	}
	env, err := s.Report(ctx, c, key, in)
	if err != nil {
		return ExportFile{}, err
	}
	ds := Dataset(env)
	var body []byte
	if f == ioengine.ExportCSV {
		body, err = ioengine.EncodeCSV(ds, env.Locale)
	} else {
		body, err = ioengine.EncodeXLSX(ds, env.Locale, nil)
	}
	if err != nil {
		return ExportFile{}, err
	}
	name := "report-" + strings.ReplaceAll(env.Report, ".", "-") + "_" + env.GeneratedAt.Format("20060102") + "." + ioengine.FileExtForExport(f)
	return ExportFile{Filename: name, ContentType: ioengine.ContentTypeForExport(f), Body: body}, nil
}

// exportType maps a report column type to an export cell type.
func exportType(t string) ioengine.ColumnType {
	switch t {
	case "number", "percent", "duration", "integer":
		return ioengine.ColumnTypeNumber
	case "money":
		return ioengine.ColumnTypeMoney
	case "datetime":
		return ioengine.ColumnTypeDatetime
	default:
		return ioengine.ColumnTypeString
	}
}

// exportColumn is a dataset column whose label is already translated (the
// encoder translates LabelKey and keeps an unknown key as is).
func exportColumn(key, label string, t ioengine.ColumnType) ioengine.Column {
	return ioengine.Column{Key: key, LabelKey: label, Type: t, AlignRight: t == ioengine.ColumnTypeNumber || t == ioengine.ColumnTypeMoney}
}

// Dataset is the export table of an envelope.
func Dataset(env Envelope) ioengine.Dataset {
	ds := ioengine.Dataset{Resource: "reports." + env.Report, Rows: []map[string]any{}, Timezone: env.Timezone}
	loc := localeOf(env)
	compared := env.PeriodInfo != nil && env.PeriodInfo.Compare != CompareNone
	switch {
	case len(env.Columns) > 0:
		hasMoney, hasCurrency := false, false
		for _, c := range env.Columns {
			ds.Columns = append(ds.Columns, exportColumn(c.Key, c.Label, exportType(c.Type)))
			if c.Type == "money" && c.Currency == nil {
				hasMoney = true
			}
			if c.Key == "currency" {
				hasCurrency = true
			}
		}
		if hasMoney && !hasCurrency {
			ds.Columns = append(ds.Columns, exportColumn("currency", translate(loc, "column.currency"), ioengine.ColumnTypeString))
		}
		for _, r := range env.Rows {
			row := map[string]any{}
			for k, v := range r {
				row[k] = v
			}
			if st, ok := r[rowStatus].(map[string]StatusCell); ok {
				for k, cell := range st {
					row[k] = cell.Label
				}
			}
			ds.Rows = append(ds.Rows, row)
		}
	case env.Kind == KindTimeseries || allTimeline(env.Series):
		ds.Columns = append(ds.Columns, exportColumn("bucket", translate(loc, "column.bucket"), ioengine.ColumnTypeString))
		for i, sr := range env.Series {
			ds.Columns = append(ds.Columns, exportColumn(seriesCol(i, ""), sr.Label, ioengine.ColumnTypeNumber))
			if compared {
				ds.Columns = append(ds.Columns, exportColumn(seriesCol(i, "p"), sr.Label+" ("+translate(loc, "column.previous")+")", ioengine.ColumnTypeNumber))
			}
		}
		var buckets []Point
		if len(env.Series) > 0 {
			buckets = env.Series[0].Points
		}
		for b, p := range buckets {
			row := map[string]any{"bucket": p.Label}
			for i, sr := range env.Series {
				if b < len(sr.Points) {
					row[seriesCol(i, "")] = sr.Points[b].Value
					if sr.Points[b].Previous != nil {
						row[seriesCol(i, "p")] = *sr.Points[b].Previous
					}
				}
			}
			ds.Rows = append(ds.Rows, row)
		}
	default:
		ds.Columns = []ioengine.Column{
			exportColumn("series", translate(loc, "column.series"), ioengine.ColumnTypeString),
			exportColumn("item", translate(loc, "column.item"), ioengine.ColumnTypeString),
			exportColumn("value", translate(loc, "column.value"), ioengine.ColumnTypeNumber),
		}
		if compared {
			ds.Columns = append(ds.Columns,
				exportColumn("previous", translate(loc, "column.previous"), ioengine.ColumnTypeNumber),
				exportColumn("delta_pct", translate(loc, "column.delta_pct"), ioengine.ColumnTypeNumber))
		}
		cardsLabel := translate(loc, "series."+seriesCards)
		for _, k := range env.KPIs {
			row := map[string]any{"series": cardsLabel, "item": k.Label, "value": k.Value}
			if k.Previous != nil {
				row["previous"] = *k.Previous
			}
			if k.DeltaPct != nil {
				row["delta_pct"] = *k.DeltaPct
			}
			ds.Rows = append(ds.Rows, row)
		}
		for _, sr := range env.Series {
			if sr.Key == seriesCards && len(env.KPIs) > 0 {
				continue
			}
			for _, p := range sr.Points {
				row := map[string]any{"series": sr.Label, "item": p.Label, "value": p.Value}
				if p.Previous != nil {
					row["previous"] = *p.Previous
					if d := deltaPct(&p.Value, p.Previous); d != nil {
						row["delta_pct"] = *d
					}
				}
				ds.Rows = append(ds.Rows, row)
			}
		}
	}
	return ds
}

func seriesCol(i int, suffix string) string {
	return "s" + string(rune('a'+i%26)) + suffix
}

func allTimeline(series []Series) bool {
	if len(series) == 0 {
		return false
	}
	for _, sr := range series {
		if !sr.timeline {
			return false
		}
	}
	return true
}

func localeOf(env Envelope) i18n.Locale { return i18n.Locale(env.Locale) }

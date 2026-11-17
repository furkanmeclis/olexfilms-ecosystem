package usecase

// TEC-1210 (analytics A): envelope v2 — compared range, series totals and
// deltas, KPIs with sparks, typed columns, row links and status tones.

import (
	"math"
)

// KPI is one headline number of a report.
type KPI struct {
	Key   string  `json:"key"`
	Label string  `json:"label"`
	Unit  string  `json:"unit"`
	Value float64 `json:"value"`
	// Previous is the value of the compared range (null with compare=none
	// and for snapshot numbers such as open orders or stock on hand, which
	// have no history).
	Previous *float64 `json:"previous"`
	// DeltaPct is the change in percent, one decimal (null without a
	// previous value or when it is 0).
	DeltaPct *float64 `json:"delta_pct"`
	// Good is the direction that is an improvement: up or down.
	Good string `json:"good"`
	// Spark is the bucket values of the period (at most the last 30; empty
	// for snapshot numbers).
	Spark []float64 `json:"spark"`
	// Link opens the detail report of the number (null: none).
	Link *Link `json:"link"`
}

// Link is a drill-down target: a record (kind + uuid) or, for a KPI, a
// report (kind=report + report key).
type Link struct {
	Kind   string `json:"kind"`
	UUID   string `json:"uuid,omitempty"`
	Report string `json:"report,omitempty"`
}

// Link kinds.
const (
	LinkService      = "service"
	LinkOrder        = "order"
	LinkCustomer     = "customer"
	LinkOrganization = "organization"
	LinkWarranty     = "warranty"
	LinkMeasurement  = "measurement"
	LinkProduct      = "product"
	LinkReport       = "report"
)

// StatusCell is the label and tone of a status column value (rows[]._status).
type StatusCell struct {
	Label string `json:"label"`
	Tone  string `json:"tone"`
}

// Status tones.
const (
	ToneNeutral  = "neutral"
	ToneProgress = "progress"
	ToneDone     = "done"
	ToneProblem  = "problem"
)

// Row meta keys.
const (
	rowLink   = "_link"
	rowStatus = "_status"
)

const (
	goodUp   = "up"
	goodDown = "down"
	unitHour = "hours"
	// maxSpark caps KPI sparks.
	maxSpark = 30
)

// kpiMeta describes a KPI: the good direction, the unit (default count),
// the report it opens and whether it is a snapshot (no history to compare).
type kpiMeta struct {
	good     string
	unit     string
	report   string
	snapshot bool
}

var kpiMetas = map[string]kpiMeta{
	"services_created":       {good: goodUp, report: ReportServicesTrend},
	"services_completed":     {good: goodUp, report: ReportServicesTrend},
	"services_open":          {good: goodDown, report: ReportServicesStatusDistribution, snapshot: true},
	"orders_created":         {good: goodUp, report: ReportOrdersTrend},
	"orders_open":            {good: goodDown, report: ReportOrdersStatusDistribution, snapshot: true},
	"customers_total":        {good: goodUp, report: ReportCustomersTrend, snapshot: true},
	"customers_new":          {good: goodUp, report: ReportCustomersTrend},
	"warranties_active":      {good: goodUp, report: ReportWarrantiesSummary, snapshot: true},
	"warranties_total":       {good: goodUp, report: ReportWarrantiesSummary, snapshot: true},
	"warranties_expiring":    {good: goodDown, report: ReportWarrantiesSummary, snapshot: true},
	"warranties_started":     {good: goodUp, report: ReportWarrantiesSummary},
	"warranties_expired":     {good: goodDown, report: ReportWarrantiesSummary, snapshot: true},
	"warranties_void":        {good: goodDown, report: ReportWarrantiesSummary, snapshot: true},
	"stock_products":         {good: goodUp, report: ReportStockSummary, snapshot: true},
	"stock_quantity":         {good: goodUp, unit: unitPieces, report: ReportStockSummary, snapshot: true},
	"stock_meters":           {good: goodUp, unit: unitMeters, report: ReportStockSummary, snapshot: true},
	"measurements_total":     {good: goodUp, report: ReportMeasurementsSummary},
	"measurements_linked":    {good: goodUp, report: ReportMeasurementsSummary},
	"measurements_vehicles":  {good: goodUp, report: ReportMeasurementsSummary},
	"dealers_total":          {good: goodUp, report: ReportNetworkSummary, snapshot: true},
	"dealers_active":         {good: goodUp, report: ReportNetworkSummary, snapshot: true},
	"distributors_total":     {good: goodUp, report: ReportNetworkSummary, snapshot: true},
	"inactive_dealers":       {good: goodDown, report: ReportOrdersInactiveBuyer},
	"fulfillment_shipped":    {good: goodUp, report: ReportOrdersFulfillment},
	"fulfillment_received":   {good: goodUp, report: ReportOrdersFulfillment},
	"fulfillment_prep_hours": {good: goodDown, unit: unitHour, report: ReportOrdersFulfillment},
	"fulfillment_open":       {good: goodDown, report: ReportOrdersFulfillment, snapshot: true},
	"fulfillment_overdue":    {good: goodDown, report: ReportOrdersFulfillment, snapshot: true},
	"network_services":       {good: goodUp, report: ReportServicesByOrg},
	"network_orders":         {good: goodUp, report: ReportOrdersByBuyer},
	"network_warranties":     {good: goodUp, report: ReportDealersTopByWarranty},
	"network_active_dealers": {good: goodUp, report: ReportNetworkSummary, snapshot: true},
}

// kpisFromCards builds the KPIs of the cards series in its order; sparks
// maps a card key to its bucket values.
func kpisFromCards(report string, cards Series, sparks map[string][]float64) []KPI {
	out := make([]KPI, 0, len(cards.Points))
	for _, p := range cards.Points {
		m, ok := kpiMetas[p.Key]
		if !ok {
			m = kpiMeta{good: goodUp}
		}
		unit := m.unit
		if unit == "" {
			unit = unitCount
		}
		k := KPI{Key: p.Key, Label: p.Label, Unit: unit, Value: p.Value, Good: m.good, Spark: []float64{}}
		if !m.snapshot {
			k.Spark = lastN(sparks[p.Key], maxSpark)
		}
		if m.report != "" && m.report != report {
			k.Link = &Link{Kind: LinkReport, Report: m.report}
		}
		out = append(out, k)
	}
	return out
}

func lastN(v []float64, n int) []float64 {
	if v == nil {
		return []float64{}
	}
	if len(v) > n {
		v = v[len(v)-n:]
	}
	out := make([]float64, len(v))
	copy(out, v)
	return out
}

func isSnapshot(key string) bool { return kpiMetas[key].snapshot }

// mergePrevious copies the compared envelope into env: series totals,
// points (timeline series by bucket index, the others by key) and KPIs
// (by key; snapshot ones stay null).
func mergePrevious(env *Envelope, prev Envelope) {
	byKey := map[string]Series{}
	for _, sr := range prev.Series {
		byKey[sr.Key] = sr
	}
	for i := range env.Series {
		sr := &env.Series[i]
		ps, ok := byKey[sr.Key]
		if !ok {
			continue
		}
		cards := sr.Key == seriesCards
		if !cards {
			t := sum(ps)
			sr.PreviousTotal = &t
		}
		if sr.timeline {
			for j := range sr.Points {
				if j < len(ps.Points) {
					v := ps.Points[j].Value
					sr.Points[j].Previous = &v
				}
			}
			continue
		}
		pv := map[string]float64{}
		for _, p := range ps.Points {
			pv[p.Key] = p.Value
		}
		for j := range sr.Points {
			p := &sr.Points[j]
			if cards && isSnapshot(p.Key) {
				continue
			}
			v, ok := pv[p.Key]
			if !ok {
				// A ranked item missing from the previous range had none.
				if cards {
					continue
				}
				v = 0
			}
			p.Previous = &v
		}
	}
	pk := map[string]float64{}
	for _, k := range prev.KPIs {
		pk[k.Key] = k.Value
	}
	for i := range env.KPIs {
		k := &env.KPIs[i]
		if isSnapshot(k.Key) {
			continue
		}
		if v, ok := pk[k.Key]; ok {
			k.Previous = &v
		}
	}
}

// finalize fills the series totals and every delta.
func finalize(env *Envelope) {
	moneyColumns(env)
	for i := range env.Series {
		sr := &env.Series[i]
		if sr.Key != seriesCards {
			t := sum(*sr)
			sr.Total = &t
		}
		sr.DeltaPct = deltaPct(sr.Total, sr.PreviousTotal)
	}
	for i := range env.KPIs {
		k := &env.KPIs[i]
		v := k.Value
		k.DeltaPct = deltaPct(&v, k.Previous)
	}
}

// moneyColumns (TEC-1214) sets the currency of each money column from the
// rows: one shared currency becomes columns[].currency; rows in different
// currencies keep their own row currency and the column gets no sum
// footer, so amounts in different currencies are never added up.
func moneyColumns(env *Envelope) {
	cur, mixed := "", false
	for _, row := range env.Rows {
		c, _ := row["currency"].(string)
		if c == "" {
			continue
		}
		if cur == "" {
			cur = c
		} else if c != cur {
			mixed = true
		}
	}
	for i := range env.Columns {
		col := &env.Columns[i]
		if col.Type != "money" {
			continue
		}
		switch {
		case mixed:
			col.Currency = nil
			if col.Aggregate == "sum" {
				col.Aggregate = "none"
			}
		case cur != "":
			c := cur
			col.Currency = &c
		}
	}
}

func sum(sr Series) float64 {
	var t float64
	for _, p := range sr.Points {
		t += p.Value
	}
	return round(t, 2)
}

// deltaPct is (cur - prev) / prev in percent with one decimal; null when
// either side is missing or prev is 0.
func deltaPct(cur, prev *float64) *float64 {
	if cur == nil || prev == nil || *prev == 0 {
		return nil
	}
	d := round((*cur-*prev)/math.Abs(*prev)*100, 1)
	return &d
}

func round(v float64, digits int) float64 {
	p := math.Pow(10, float64(digits))
	return math.Round(v*p) / p
}

// Status tones of the record statuses a table shows.
var statusTones = map[string]map[string]string{
	"services": {
		"draft": ToneNeutral, "pending": ToneNeutral, "processing": ToneProgress, "ready": ToneProgress,
		"completed": ToneDone, "cancelled": ToneProblem,
	},
	"organizations": {
		"pending": ToneNeutral, "active": ToneDone, "read_only": ToneProblem, "suspended": ToneProblem, "expired": ToneProblem,
	},
}

// statusCell is the _status entry of a domain status.
func (q query) statusCell(domain, labelPrefix, status string) StatusCell {
	tone, ok := statusTones[domain][status]
	if !ok {
		tone = ToneNeutral
	}
	return StatusCell{Label: translate(q.locale, labelPrefix+status), Tone: tone}
}

func link(kind string, id string) Link { return Link{Kind: kind, UUID: id} }

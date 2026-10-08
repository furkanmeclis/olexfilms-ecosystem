// Package model holds the performance module's keys (TEC-490, F5-05a).
package model

// Monthly metric keys of performance_metrics_monthly (CHECK in migration
// 000123). Money metrics are in the organization currency.
const (
	MetricServicesCount       = "services_count"
	MetricWarrantyStartRate   = "warranty_start_rate"
	MetricMeasurementRate     = "measurement_rate"
	MetricReviewAvg           = "review_avg"
	MetricStockTurnover       = "stock_turnover"
	MetricContractDaysLeft    = "contract_days_left"
	MetricCariOverdueAmount   = "cari_overdue_amount"
	MetricCariOverdueDays     = "cari_overdue_days"
	MetricCertificateCoverage = "certificate_coverage"
	MetricLeadConversionRate  = "lead_conversion_rate"
	MetricWasteRatio          = "waste_ratio"
	MetricOrderVolume         = "order_volume"
)

// Metrics lists every monthly metric key in display order.
var Metrics = []string{
	MetricServicesCount, MetricWarrantyStartRate, MetricMeasurementRate, MetricReviewAvg,
	MetricStockTurnover, MetricContractDaysLeft, MetricCariOverdueAmount, MetricCariOverdueDays,
	MetricCertificateCoverage, MetricLeadConversionRate, MetricWasteRatio, MetricOrderVolume,
}

// MoneyMetrics carry a currency (organization currency).
var MoneyMetrics = []string{MetricOrderVolume, MetricCariOverdueAmount}

// IsMetric reports whether key is a monthly metric key.
func IsMetric(key string) bool {
	for _, m := range Metrics {
		if m == key {
			return true
		}
	}
	return false
}

// Network target metrics (performance_targets.metric).
var TargetMetrics = []string{MetricServicesCount, MetricOrderVolume}

// Target period kinds.
const (
	PeriodMonthly   = "monthly"
	PeriodQuarterly = "quarterly"
	PeriodYearly    = "yearly"
)

var PeriodKinds = []string{PeriodMonthly, PeriodQuarterly, PeriodYearly}

// Staff target and bonus rule metrics (inside a dealer).
const MetricServiceRevenue = "service_revenue"

var StaffMetrics = []string{MetricServicesCount, MetricServiceRevenue}

// Bonus rule kinds.
const (
	BonusFixed            = "fixed"
	BonusPercentOfRevenue = "percent_of_revenue"
)

var BonusKinds = []string{BonusFixed, BonusPercentOfRevenue}

// Bonus accrual statuses.
const (
	AccrualCalculated = "calculated"
	AccrualApproved   = "approved"
	AccrualPosted     = "posted"
	AccrualCancelled  = "cancelled"
)

var AccrualStatuses = []string{AccrualCalculated, AccrualApproved, AccrualPosted, AccrualCancelled}

// Weak dealer rules: target_achievement is the achievement % of the active
// service count target; other rule metrics are the monthly metric keys.
const RuleMetricTargetAchievement = "target_achievement"

// Weak dealer rule operators. below_median_pct matches a value at least
// threshold % under the median of the rule's network.
const (
	OpLT             = "lt"
	OpLTE            = "lte"
	OpGT             = "gt"
	OpGTE            = "gte"
	OpBelowMedianPct = "below_median_pct"
)

var RuleOperators = []string{OpLT, OpLTE, OpGT, OpGTE, OpBelowMedianPct}

// RuleMetrics lists the metric keys a weak dealer rule may watch.
func RuleMetrics() []string {
	return append([]string{RuleMetricTargetAchievement}, Metrics...)
}

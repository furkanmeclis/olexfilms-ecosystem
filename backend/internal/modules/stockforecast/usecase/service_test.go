package usecase

import (
	"math"
	"testing"
	"time"
)

func TestCalculateMovingAverageThresholdsAndSuggestion(t *testing.T) {
	in := calcInput{
		OrganizationID: 1, BrandID: 2, ProductID: 3,
		ComputedOn: time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC),
		OnHandQty:  10, Avg30: 2, Avg90: 1, Seasonality: 1.5,
		DataDays: 120, MinDays: 90, WarningDays: 14, CriticalDays: 7, CoverDays: 30,
	}
	got := calculate(in)
	speed := (0.6*2 + 0.4*1) * 1.5
	wantDays := 10 / speed
	if got.Status != StatusCritical {
		t.Fatalf("status = %s, want critical", got.Status)
	}
	if !near(numeric(got.DaysLeft), wantDays) {
		t.Fatalf("days_left = %.4f, want %.4f", numeric(got.DaysLeft), wantDays)
	}
	if got.SuggestedQty.Int32 != int32(math.Ceil(speed*30-10)) {
		t.Fatalf("suggested = %d", got.SuggestedQty.Int32)
	}
}

func TestCalculateInsufficientDataProducesNoSuggestion(t *testing.T) {
	got := calculate(calcInput{
		ComputedOn: time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC),
		OnHandQty:  1, Avg30: 10, Avg90: 10, Seasonality: 2,
		DataDays: 20, MinDays: 90, WarningDays: 14, CriticalDays: 7, CoverDays: 30,
	})
	if got.Status != StatusInsufficientData {
		t.Fatalf("status = %s", got.Status)
	}
	if got.SuggestedQty.Valid || got.SuggestedMeters.Valid || got.DaysLeft.Valid {
		t.Fatalf("insufficient data must not propose: %+v", got)
	}
}

func TestCalculateNoConsumption(t *testing.T) {
	got := calculate(calcInput{
		ComputedOn: time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC),
		OnHandQty:  25, DataDays: 120, MinDays: 90, WarningDays: 14, CriticalDays: 7, CoverDays: 30,
	})
	if got.Status != StatusNoConsumption || got.DaysLeft.Valid || got.SuggestedQty.Valid {
		t.Fatalf("no consumption result = %+v", got)
	}
}

func TestCalculateRollMetersVehiclesLeft(t *testing.T) {
	got := calculate(calcInput{
		ComputedOn: time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC),
		UnitType:   "roll_meter", OnHandMeters: 50, OpenOrderMeters: 10,
		Avg30: 2, Avg90: 1, Seasonality: 1,
		DataDays: 120, MinDays: 90, WarningDays: 14, CriticalDays: 7, CoverDays: 30,
		PartialMeters90: 45, Services90: 9,
	})
	if !near(numeric(got.AvgMetersPerVehicle), 5) || !near(numeric(got.VehiclesLeft), 10) {
		t.Fatalf("meter vehicle fields = avg %.2f vehicles %.2f", numeric(got.AvgMetersPerVehicle), numeric(got.VehiclesLeft))
	}
	if !got.SuggestedMeters.Valid || got.SuggestedQty.Valid {
		t.Fatalf("roll must suggest meters only: %+v", got)
	}
}

func TestShouldNotifyOnlyThresholdTransitions(t *testing.T) {
	tests := []struct {
		name       string
		prev, next string
		want       bool
	}{
		{name: "ok to warning", prev: StatusOK, next: StatusWarning, want: true},
		{name: "warning to critical", prev: StatusWarning, next: StatusCritical, want: true},
		{name: "repeat warning", prev: StatusWarning, next: StatusWarning},
		{name: "ok to critical is not named transition", prev: StatusOK, next: StatusCritical},
		{name: "insufficient", prev: StatusOK, next: StatusInsufficientData},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldNotify(tt.prev, tt.next); got != tt.want {
				t.Fatalf("shouldNotify(%s,%s) = %v", tt.prev, tt.next, got)
			}
		})
	}
}

func TestSeasonalityClamp(t *testing.T) {
	if got := clamp(0.1, 0.5, 2); got != 0.5 {
		t.Fatalf("low clamp = %v", got)
	}
	if got := clamp(3, 0.5, 2); got != 2 {
		t.Fatalf("high clamp = %v", got)
	}
}

func TestPerformanceFixtureShape(t *testing.T) {
	start := time.Now()
	var critical int
	for org := 0; org < 500; org++ {
		for product := 0; product < 120; product++ {
			got := calculate(calcInput{
				ComputedOn: time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC),
				OnHandQty:  int32(20 + product%7), Avg30: float64(1 + product%5), Avg90: 1.25, Seasonality: 1.1,
				DataDays: 120, MinDays: 90, WarningDays: 14, CriticalDays: 7, CoverDays: 30,
			})
			if got.Status == StatusCritical {
				critical++
			}
		}
	}
	t.Logf("stock forecast performance fixture: 500 org x 120 product calculated in %s; critical=%d", time.Since(start), critical)
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.015 }

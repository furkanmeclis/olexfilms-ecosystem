package usecase

import (
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestCountAllowedResolutions(t *testing.T) {
	cases := []struct {
		kind, result string
		want         []string
	}{
		{CountScanSerial, CountResultMatched, []string{}},
		{CountScanSerial, CountResultMissing, []string{CountResolveIgnore, CountResolveVoidMissing}},
		{CountScanSerial, CountResultWrongLocation, []string{CountResolveIgnore, CountResolveRelocate}},
		{CountScanSerial, CountResultUnlocated, []string{CountResolveIgnore, CountResolveRelocate}},
		{CountScanSerial, CountResultUnexpected, []string{CountResolveIgnore}},
		{CountScanSerial, CountResultMeterVariance, []string{CountResolveIgnore, CountResolveIncreaseUnlocated}},
		{CountScanFixed, CountResultQtyVariance, []string{CountResolveIgnore, CountResolveIncreaseUnlocated}},
		// Serial products counted by quantity cannot be corrected without
		// knowing the units: report only.
		{CountScanProduct, CountResultQtyVariance, []string{CountResolveIgnore}},
	}
	for _, c := range cases {
		if got := allowedResolutions(c.kind, c.result); !slices.Equal(got, c.want) {
			t.Errorf("%s/%s = %v, want %v", c.kind, c.result, got, c.want)
		}
	}
}

func TestCountCSVSafe(t *testing.T) {
	for in, want := range map[string]string{"": "", "ABC-1": "ABC-1", "=SUM(A1)": "'=SUM(A1)", "-5": "'-5", "@x": "'@x"} {
		if got := csvSafe(in); got != want {
			t.Errorf("csvSafe(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCountMeters(t *testing.T) {
	n := cmToNum(850)
	if cm, ok := numToCm(n); !ok || cm != 850 {
		t.Fatalf("round trip = %d %v", cm, ok)
	}
	if _, ok := numToCm(pgtype.Numeric{}); ok {
		t.Fatal("NULL numeric converted")
	}
}

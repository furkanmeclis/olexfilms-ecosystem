package usecase

import (
	"github.com/google/uuid"
	"testing"
)

func TestScanStatus(t *testing.T) {
	warehouse := uuid.New()
	other := uuid.New()
	bin, aisle := "bin", "aisle"
	cases := []struct {
		name      string
		result    ScanResult
		expect    string
		warehouse uuid.UUID
		want      string
	}{
		{"location", ScanResult{Type: ScanTypeLocation, Location: &ScanLocation{Type: &bin, Path: []ScanPathNode{{Level: "warehouse", UUID: warehouse}}}}, "location", warehouse, "ok"},
		{"bin", ScanResult{Type: ScanTypeLocation, Location: &ScanLocation{Type: &bin}}, "bin", uuid.Nil, "ok"},
		{"wrong type", ScanResult{Type: ScanTypeLocation, Location: &ScanLocation{Type: &bin}}, "unit", uuid.Nil, "type_mismatch"},
		{"not bin", ScanResult{Type: ScanTypeLocation, Location: &ScanLocation{Type: &aisle}}, "bin", uuid.Nil, "not_bin"},
		{"wrong warehouse", ScanResult{Type: ScanTypeLocation, Location: &ScanLocation{Type: &bin, Path: []ScanPathNode{{Level: "warehouse", UUID: other}}}}, "bin", warehouse, "warehouse_mismatch"},
		{"void", ScanResult{Type: ScanTypeUnit, Unit: &ScanUnit{Status: "void"}}, "unit", uuid.Nil, "void"},
		{"unit", ScanResult{Type: ScanTypeUnit, Unit: &ScanUnit{Status: "placed"}}, "unit", uuid.Nil, "ok"},
		{"product", ScanResult{Type: ScanTypeProduct}, "product", uuid.Nil, "ok"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scanStatus(tc.result, tc.expect, tc.warehouse); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestShortCodePattern(t *testing.T) {
	for code, suffix := range map[string]string{
		"123": "", "123-S2": "-S2", "123-3M": "-3M", "123-2.5M": "-2.5M", "123-3M-2": "-3M-2", "12345678-12.05M-10": "-12.05M-10",
	} {
		m := shortCodePattern.FindStringSubmatch(code)
		if m == nil || m[2] != suffix {
			t.Fatalf("%s: %v, want suffix %q", code, m, suffix)
		}
	}
	for _, code := range []string{"123-M", "123-3", "123-3M-", "123-2.555M", "123-S", "ABC-3M"} {
		if shortCodePattern.MatchString(code) {
			t.Fatalf("%s must not match", code)
		}
	}
}

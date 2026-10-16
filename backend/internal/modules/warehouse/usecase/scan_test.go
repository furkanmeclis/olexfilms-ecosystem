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

package source

import "testing"

func TestDSNSetsLoc(t *testing.T) {
	cases := map[string]bool{
		"u:p@tcp(h:3306)/db":                                false,
		"u:p@tcp(h:3306)/db?parseTime=true":                 false,
		"u:p@tcp(h:3306)/db?loc=UTC":                        true,
		"u:p@tcp(h:3306)/db?parseTime=true&loc=Local":       true,
		"u:p@tcp(h:3306)/db?charset=utf8mb4&allowloc=false": false,
	}
	for dsn, want := range cases {
		if got := dsnSetsLoc(dsn); got != want {
			t.Errorf("dsnSetsLoc(%q) = %v, want %v", dsn, got, want)
		}
	}
}

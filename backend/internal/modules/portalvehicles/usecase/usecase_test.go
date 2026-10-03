package usecase

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestTimeLeft(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 100)
	cases := []struct {
		name      string
		now       time.Time
		days, pct int
	}{
		{"before start", start.Add(-time.Hour), 101, 100},
		{"at start", start, 100, 100},
		{"half way", start.AddDate(0, 0, 50), 50, 50},
		{"partial day rounds up", end.Add(-time.Hour), 1, 0},
		{"at end", end, 0, 0},
		{"after end", end.AddDate(0, 0, 3), 0, 0},
	}
	for _, c := range cases {
		days, pct := TimeLeft(start, end, c.now)
		if days != c.days || pct != c.pct {
			t.Errorf("%s: got %d days %d%%, want %d days %d%%", c.name, days, pct, c.days, c.pct)
		}
	}
	if d, p := TimeLeft(end, start, start); d != 0 || p != 0 {
		t.Errorf("inverted period: %d %d", d, p)
	}
}

// The portal views carry no measurement data (TEC-238 acceptance).
func TestViewsHaveNoMeasurement(t *testing.T) {
	v := VehicleDetailView{
		Services:         []ServiceView{{}},
		ActiveWarranties: []ActiveWarrantyView{{}},
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(b)), "measurement") {
		t.Fatalf("view carries measurement data: %s", b)
	}
}

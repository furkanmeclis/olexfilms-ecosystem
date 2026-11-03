package popularity

import (
	"math"
	"reflect"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// 60 % organization + 40 % general, each normalised by its own maximum.
func TestScoresMixed(t *testing.T) {
	org := Counts{1: 10, 2: 5}
	global := Counts{1: 20, 2: 100, 3: 50}
	s := Scores(org, global, true)
	want := map[int64]float64{
		1: 0.6*10.0/10 + 0.4*20.0/100, // 0.68
		2: 0.6*5.0/10 + 0.4*100.0/100, // 0.70
		3: 0.6*0 + 0.4*50.0/100,       // 0.20
	}
	if len(s) != len(want) {
		t.Fatalf("scores = %v, want %v", s, want)
	}
	for id, w := range want {
		if !near(s[id], w) {
			t.Fatalf("score[%d] = %v, want %v", id, s[id], w)
		}
	}
	// The general count can outweigh a smaller organization lead.
	if !(s[2] > s[1] && s[1] > s[3]) {
		t.Fatalf("order of scores %v", s)
	}
}

// A caller without an organization gets the general count alone, even when
// organization counts are passed in by mistake.
func TestScoresWithoutOrg(t *testing.T) {
	s := Scores(Counts{9: 1000}, Counts{1: 4, 2: 2}, false)
	if _, ok := s[9]; ok {
		t.Fatalf("org-only id scored without an organization: %v", s)
	}
	if !near(s[1], 1) || !near(s[2], 0.5) {
		t.Fatalf("scores = %v, want 1 and 0.5", s)
	}
}

// Empty data: no scores, no division by zero; the max is at least 1.
func TestScoresEmptyAndMaxFloor(t *testing.T) {
	if s := Scores(nil, nil, true); len(s) != 0 {
		t.Fatalf("empty scores = %v", s)
	}
	if s := Scores(Counts{}, Counts{}, false); len(s) != 0 {
		t.Fatalf("empty scores = %v", s)
	}
	s := Scores(Counts{1: 0}, Counts{1: 0}, true)
	if v := s[1]; math.IsNaN(v) || v != 0 {
		t.Fatalf("zero counts score = %v, want 0", v)
	}
	// A new organization (no own usage) still ranks by the general count.
	s = Scores(Counts{}, Counts{1: 3, 2: 1}, true)
	if !near(s[1], 0.4) || !near(s[2], 0.4/3) {
		t.Fatalf("new organization scores = %v", s)
	}
}

// Ties: score, then case-insensitive name, then id; unused rows last.
func TestRankTiebreak(t *testing.T) {
	entries := []Entry{{ID: 5, Name: "bmw"}, {ID: 4, Name: "Audi"}, {ID: 3, Name: "BMW"}, {ID: 2, Name: "Citroen"}, {ID: 1, Name: "Zeta"}}
	scores := map[int64]float64{2: 0.5, 3: 0.2, 4: 0.2, 5: 0.2}
	got := Rank(entries, scores)
	ids := make([]int64, len(got))
	for i, e := range got {
		ids[i] = e.ID
	}
	if want := []int64{2, 4, 3, 5, 1}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("rank = %v, want %v", ids, want)
	}
	if entries[0].ID != 5 {
		t.Fatal("Rank changed its input")
	}
}

func TestArrays(t *testing.T) {
	ids, vals := Arrays(map[int64]float64{7: 0.1, 3: 0.9})
	if !reflect.DeepEqual(ids, []int64{3, 7}) || !reflect.DeepEqual(vals, []float64{0.9, 0.1}) {
		t.Fatalf("arrays = %v %v", ids, vals)
	}
	if ids, vals := Arrays(nil); ids != nil || vals != nil {
		t.Fatalf("empty arrays = %v %v", ids, vals)
	}
}

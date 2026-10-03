package appversion

import "testing"

func TestParse(t *testing.T) {
	ok := map[string]string{
		"2.4.1":        "2.4.1",
		"v2.4":         "2.4.0",
		" 3 ":          "3.0.0",
		"2.4.1+57":     "2.4.1",
		"2.4.1-beta.2": "2.4.1",
		"1.2.3.4":      "1.2.3.4",
	}
	for in, want := range ok {
		v, good := Parse(in)
		if !good || v.String() != want {
			t.Errorf("Parse(%q) = %v, %v; want %s", in, v, good, want)
		}
	}
	for _, in := range []string{"", "x", "1..2", "1.2.3.4.5", "1.a", "-1", "123456", "v"} {
		if _, good := Parse(in); good {
			t.Errorf("Parse(%q) accepted", in)
		}
	}
}

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"2.4.0", "2.4", 0},
		{"2.3.9", "2.4.0", -1},
		{"2.10.0", "2.9.9", 1},
		{"2.4.0-rc1", "2.4.0", 0},
		{"2.4.0.1", "2.4.0", 1},
	}
	for _, c := range cases {
		a, _ := Parse(c.a)
		b, _ := Parse(c.b)
		if got := Compare(a, b); got != c.want {
			t.Errorf("Compare(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestFromUserAgent(t *testing.T) {
	products := []string{"OlexFilms"}
	cases := map[string]string{
		"OlexFilms/2.4.1 (iPhone; iOS 17.4) CFNetwork/1494": "2.4.1",
		"olexfilms/v1.9 okhttp/4.12.0":                      "1.9.0",
	}
	for ua, want := range cases {
		v, ok := FromUserAgent(ua, products)
		if !ok || v.String() != want {
			t.Errorf("FromUserAgent(%q) = %v, %v; want %s", ua, v, ok, want)
		}
	}
	for _, ua := range []string{"", "okhttp/4.12.0", "Mozilla/5.0 (iPhone) CFNetwork/1494 Darwin/23.4.0", "OlexFilmsX/2.0"} {
		if _, ok := FromUserAgent(ua, products); ok {
			t.Errorf("FromUserAgent(%q) matched", ua)
		}
	}
	if _, ok := FromUserAgent("OlexFilms/2.0", nil); ok {
		t.Error("no products must not match")
	}
}

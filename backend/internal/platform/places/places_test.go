package places

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRatingRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/places/ChIJabc" || r.Header.Get("X-Goog-Api-Key") != "k" ||
			r.Header.Get("X-Goog-FieldMask") != "rating,userRatingCount" {
			t.Errorf("request: %s %v", r.URL.Path, r.Header)
		}
		_, _ = w.Write([]byte(`{"rating":4.7,"userRatingCount":128}`))
	}))
	defer srv.Close()
	r, err := New("k", WithBaseURL(srv.URL)).Rating(context.Background(), "ChIJabc")
	if err != nil || r.Rating == nil || *r.Rating != 4.7 || r.ReviewCount != 128 {
		t.Fatalf("rating = %+v, %v", r, err)
	}
}

func TestRatingErrors(t *testing.T) {
	if _, err := New(" ").Rating(context.Background(), "x"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("no key: %v", err)
	}
	for code, want := range map[int]error{429: ErrQuota, 404: ErrNotFound, 400: ErrNotFound} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) }))
		if _, err := New("k", WithBaseURL(srv.URL)).Rating(context.Background(), "ChIJ"); !errors.Is(err, want) {
			t.Errorf("%d: %v", code, err)
		}
		srv.Close()
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) }))
	defer srv.Close()
	var se *StatusError
	if _, err := New("k", WithBaseURL(srv.URL)).Rating(context.Background(), "ChIJ"); !errors.As(err, &se) || se.Code != 503 {
		t.Fatalf("503: %v", err)
	}
}

func TestParseBusinessURL(t *testing.T) {
	cases := map[string]Ref{
		"https://search.google.com/local/writereview?placeid=ChIJN1t_tDeuEmsRUsoyG83frY4": {PlaceID: "ChIJN1t_tDeuEmsRUsoyG83frY4"},
		"https://www.google.com/maps/search/?api=1&query=x&query_place_id=ChIJ-abc_1":     {PlaceID: "ChIJ-abc_1"},
		"https://www.google.com/maps/place/?q=place_id:ChIJxyz":                           {PlaceID: "ChIJxyz"},
		"https://maps.google.com/?cid=1234567890123":                                      {CID: "1234567890123"},
		"https://g.page/r/abc/review":                                                     {},
		"https://www.google.com/maps/place/Olex+Films/@39.9,32.8,17z":                     {},
		"https://search.google.com/local/writereview?placeid=bad%20id":                    {},
		"not a url": {},
	}
	for in, want := range cases {
		if got := ParseBusinessURL(in); got != want {
			t.Errorf("%s: %+v, want %+v", in, got, want)
		}
	}
}

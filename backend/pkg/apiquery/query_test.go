package apiquery_test

import (
	"net/url"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
)

func TestParseAndValidateSort(t *testing.T) {
	t.Parallel()
	q := apiquery.Parse(url.Values{
		"limit":  []string{"50"},
		"offset": []string{"10"},
		"q":      []string{"acme"},
		"sort":   []string{"-created_at,name"},
	})
	if q.Limit != 50 || q.Offset != 10 || q.Q != "acme" {
		t.Fatalf("parse: %+v", q)
	}
	if err := apiquery.ValidateSort(q.Sort, apiquery.TenantsSort); err != nil {
		t.Fatal(err)
	}
	if err := apiquery.ValidateSort(apiquery.ParseSort("bogus"), apiquery.TenantsSort); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestForbiddenParams(t *testing.T) {
	t.Parallel()
	got := apiquery.ForbiddenParams(url.Values{"page": []string{"1"}, "search": []string{"x"}})
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
}

func TestFromSlice(t *testing.T) {
	t.Parallel()
	page := apiquery.FromSlice([]int{1, 2, 3, 4, 5}, 2, 1)
	if page.Total != 5 || len(page.Items) != 2 || page.Items[0] != 2 {
		t.Fatalf("%+v", page)
	}
}

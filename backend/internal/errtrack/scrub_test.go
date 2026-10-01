package errtrack

import (
	"strings"
	"testing"

	"github.com/getsentry/sentry-go"
)

func TestScrubString(t *testing.T) {
	cases := map[string]string{
		"mail ali.veli+x@olexfilms.app failed":       "mail [email] failed",
		"phone +905551234567 invalid":                "phone [phone] invalid",
		"call 0555 123 45 67 now":                    "call [phone] now",
		"tel (0212) 555-12-34":                       "tel [phone]",
		"to:+49 30 1234567":                          "to:[phone]",
		"order 550e8400-e29b-41d4-a716-446655440000": "order 550e8400-e29b-41d4-a716-446655440000",
		"at 2026-10-01 12:00:00":                     "at 2026-10-01 12:00:00",
		"id 12345 rows 678":                          "id 12345 rows 678",
		"":                                           "",
	}
	for in, want := range cases {
		if got := ScrubString(in); got != want {
			t.Errorf("ScrubString(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestScrubEvent(t *testing.T) {
	ev := &sentry.Event{
		Message:    "user ali@example.com",
		ServerName: "host-1",
		User: sentry.User{
			ID: "u-1", Email: "ali@example.com", Name: "Ali Veli",
			Username: "aliveli", IPAddress: "1.2.3.4",
			Data: map[string]string{"phone": "+905551234567"},
		},
		Exception: []sentry.Exception{{Type: "*errors.errorString", Value: "sms to +905551234567 failed"}},
		Breadcrumbs: []*sentry.Breadcrumb{{
			Message: "login ali@example.com",
			Data:    map[string]any{"name": "Ali Veli", "count": 3, "note": "+90 555 123 45 67"},
		}},
		Tags: map[string]string{"module": "auth", "email": "ali@example.com", "organization_id": "6f1c1c1e-1111-4a2b-9c3d-000000000001"},
		Contexts: map[string]sentry.Context{
			"customer": {"full_name": "Ali Veli", "nested": map[string]any{"phone": "1"}, "city": "Ankara"},
			"runtime":  {"name": "go"},
		},
		Request: &sentry.Request{
			URL: "https://olexfilms.app/api/v1/customers/+905551234567?email=a@b.co", Method: "GET",
			Cookies: "session=secret", Headers: map[string]string{"Authorization": "Bearer x"},
			Data: `{"phone":"+905551234567"}`, QueryString: "email=a@b.co",
			Env: map[string]string{"REMOTE_ADDR": "1.2.3.4"},
		},
	}
	out := beforeSend(ev, nil)
	if out.User.ID != "u-1" || out.User.Email != "" || out.User.Name != "" || out.User.Username != "" ||
		out.User.IPAddress != "" || len(out.User.Data) != 0 {
		t.Errorf("user must keep only id: %+v", out.User)
	}
	if out.Message != "user [email]" || out.ServerName != "" {
		t.Errorf("message/server = %q/%q", out.Message, out.ServerName)
	}
	if out.Exception[0].Value != "sms to [phone] failed" {
		t.Errorf("exception = %q", out.Exception[0].Value)
	}
	bc := out.Breadcrumbs[0]
	if bc.Message != "login [email]" || bc.Data["name"] != maskFiltered || bc.Data["count"] != 3 || bc.Data["note"] != "[phone]" {
		t.Errorf("breadcrumb = %+v", bc)
	}
	if out.Tags["email"] != maskFiltered || out.Tags["module"] != "auth" ||
		out.Tags["organization_id"] != "6f1c1c1e-1111-4a2b-9c3d-000000000001" {
		t.Errorf("tags = %v", out.Tags)
	}
	cust := out.Contexts["customer"]
	if cust["full_name"] != maskFiltered || cust["city"] != "Ankara" {
		t.Errorf("context = %v", cust)
	}
	if nested, _ := cust["nested"].(map[string]any); nested["phone"] != maskFiltered {
		t.Errorf("nested = %v", cust["nested"])
	}
	if out.Contexts["runtime"]["name"] != "go" {
		t.Errorf("sdk context mangled: %v", out.Contexts["runtime"])
	}
	r := out.Request
	if r.Method != "GET" || r.URL != "https://olexfilms.app/api/v1/customers/[phone]" {
		t.Errorf("request url = %q", r.URL)
	}
	if r.Cookies != "" || len(r.Headers) != 0 || r.Data != "" || r.QueryString != "" || len(r.Env) != 0 {
		t.Errorf("request must keep only method+url: %+v", r)
	}
	if strings.Contains(out.Exception[0].Value, "555") {
		t.Error("phone leaked")
	}
	if Scrub(nil) != nil {
		t.Error("nil event")
	}
}

package msgtemplate

import "testing"

func TestLocaleSamples(t *testing.T) {
	todo, _ := Lookup("todo.reminder")
	cases := map[string]string{"tr": "15 dakika sonra", "en": "in 15 minutes", "ar": "بعد 15 دقيقة", "zh-CN": "15分钟后", "de": "in 15 minutes"}
	for loc, want := range cases {
		if got := todo.SampleVars(loc)["due_in"]; got != want {
			t.Errorf("%s due_in = %q, want %q", loc, got, want)
		}
	}
	// Placeholders without a localized sample use the en one.
	if got := phPlate.Sample("bg"); got != "34 ABC 123" {
		t.Errorf("bg plate = %q", got)
	}
	for key, byLocale := range localeSamples {
		for _, loc := range []string{"bg", "el", "zh-CN", "az", "ar"} {
			if byLocale[loc] == "" {
				t.Errorf("%s: no %s sample", key, loc)
			}
		}
	}
}

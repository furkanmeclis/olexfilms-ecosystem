package msgtemplate

import (
	"reflect"
	"testing"
)

func TestRender(t *testing.T) {
	vars := map[string]string{"customer_name": "Ahmet", "quote_number": "TKL-1", "evil": "{{customer_name}}"}
	cases := map[string]string{
		"Merhaba {{customer_name}}":               "Merhaba Ahmet",
		"{{ customer_name }} / {{.quote_number}}": "Ahmet / TKL-1",
		"missing: [{{valid_until}}]":              "missing: []",
		"no nested: {{evil}}":                     "no nested: {{customer_name}}",
		"plain text":                              "plain text",
		"{{CUSTOMER_NAME}}":                       "Ahmet",
		"{{ not closed":                           "{{ not closed",
		"{{a-b}} stays":                           "{{a-b}} stays",
	}
	for in, want := range cases {
		if got := Render(in, vars); got != want {
			t.Errorf("Render(%q)=%q want %q", in, got, want)
		}
	}
}

func TestPlaceholdersAndUnknown(t *testing.T) {
	got := Placeholders("{{b}} {{a}} {{ a }} {{.c}}")
	if !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("placeholders %v", got)
	}
	spec, ok := Lookup("quote.sent")
	if !ok {
		t.Fatal("quote.sent missing")
	}
	unk := Unknown(spec, "{{quote_number}} {{foo}}", "{{bar}} {{foo}}")
	if !reflect.DeepEqual(unk, []string{"bar", "foo"}) {
		t.Fatalf("unknown %v", unk)
	}
}

func TestRegistry(t *testing.T) {
	for _, typ := range []string{"quote.created", "quote.sent", "quote.reminder", "quote.expiring", "todo.reminder", "job.created"} {
		if _, ok := Lookup(typ); !ok {
			t.Errorf("type %s not registered", typ)
		}
	}
	todo, _ := Lookup("todo.reminder")
	if !todo.UserPreference || !todo.HasChannel(ChannelInapp) {
		t.Fatal("todo.reminder must be a user-preference type with inapp")
	}
	if v := todo.SampleVars("en")["due_in"]; v != "in 15 minutes" {
		t.Fatalf("sample %q", v)
	}
	if v := todo.SampleVars("de")["due_in"]; v != "in 15 Minuten" {
		t.Fatalf("de sample %q", v)
	}
	if v := todo.SampleVars("ru-RU")["todo_link"]; v != "https://olexfilms.app/t/tech-oto/todos" {
		t.Fatalf("ru link sample should fall back to en, got %q", v)
	}
	if v := todo.SampleVars("bg")["due_in"]; v != "след 15 минути" {
		t.Fatalf("bg sample %q", v)
	}
	if v := todo.SampleVars("ar")["todo_link"]; v != "https://olexfilms.app/t/tech-oto/todos" {
		t.Fatalf("ar link sample should fall back to en, got %q", v)
	}
	if len(UserPreferenceTypes()) == 0 {
		t.Fatal("no preference types")
	}
}

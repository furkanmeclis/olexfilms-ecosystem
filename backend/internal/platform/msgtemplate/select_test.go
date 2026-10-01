package msgtemplate

import (
	"reflect"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
)

func TestLocaleChain(t *testing.T) {
	cases := []struct {
		user, org, center string
		want              []i18n.Locale
	}{
		{"ar", "tr-TR", "tr", []i18n.Locale{"ar", "tr", "en"}},
		{"", "de-DE", "", []i18n.Locale{"de", "en", "tr"}},
		{"xx", "", "", []i18n.Locale{"en", "tr"}},
		{"zh_CN", "", "en", []i18n.Locale{"zh-CN", "en", "tr"}},
	}
	for _, c := range cases {
		if got := LocaleChain(c.user, c.org, c.center); !reflect.DeepEqual(got, c.want) {
			t.Errorf("LocaleChain(%q,%q,%q) = %v, want %v", c.user, c.org, c.center, got, c.want)
		}
	}
	// The head of the chain matches i18n.Resolve when a source is set.
	if LocaleChain("", "tr-TR", "")[0] != i18n.Resolve(i18n.Sources{OrgLocale: "tr-TR"}).Locale {
		t.Fatal("chain head differs from i18n.Resolve")
	}
}

func TestPick(t *testing.T) {
	v := []Variant{
		{Role: "generic", Language: "tr"},
		{Role: "dealer", Language: "tr"},
		{Role: "generic", Language: "en"},
		{Role: "generic", Language: "en", Branded: true},
		{Role: "center", Language: "ar"},
	}
	chain := func(l ...i18n.Locale) []i18n.Locale { return l }
	cases := []struct {
		role  string
		chain []i18n.Locale
		want  int
	}{
		{"dealer", chain("tr", "en"), 1},       // role template in user language
		{"customer", chain("tr", "en"), 0},     // generic fallback, same language
		{"dealer", chain("ar", "en", "tr"), 3}, // no ar for dealer -> en, brand override first
		{"center", chain("ar", "en", "tr"), 4}, // role + ar
		{"distributor", chain("de", "en", "tr"), 3},
	}
	for _, c := range cases {
		got, ok := Pick(v, c.role, c.chain)
		if !ok || got != c.want {
			t.Errorf("Pick(%s, %v) = %d,%v want %d", c.role, c.chain, got, ok, c.want)
		}
	}
	if _, ok := Pick(nil, "dealer", chain("tr")); ok {
		t.Fatal("empty variants must not match")
	}
}

func TestMarkdownToHTML(t *testing.T) {
	got := MarkdownToHTML("# Başlık\n\n**kalın** ve *eğik* `kod`\nikinci satır\n\n- a\n- b\n\n[link](https://olexfilms.app) [kötü](javascript:alert(1)) <b>x</b>")
	for _, want := range []string{
		"<h2>Başlık</h2>", "<strong>kalın</strong>", "<em>eğik</em>", "<code>kod</code>", "<br>ikinci satır",
		"<ul><li>a</li><li>b</li></ul>", `<a href="https://olexfilms.app">link</a>`, "&lt;b&gt;x&lt;/b&gt;",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if strings.Contains(got, "javascript:") && strings.Contains(got, `href="javascript`) {
		t.Fatalf("unsafe link rendered: %s", got)
	}
	if MarkdownToText("**a** [b](https://x)") != "a b (https://x)" {
		t.Fatalf("text = %q", MarkdownToText("**a** [b](https://x)"))
	}
}

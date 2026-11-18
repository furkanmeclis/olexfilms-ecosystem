package emailpreview

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
)

// TEC-1235: every e-mail renders in every locale without leftover
// placeholders, with a subject, a preheader, lang/dir on the HTML and a
// plain-text alternative.
func TestEveryEmailRendersInEveryLocale(t *testing.T) {
	samples, err := Build(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]map[string]bool{}
	for _, s := range samples {
		id := s.Type + "/" + s.Locale
		if seen[s.Type] == nil {
			seen[s.Type] = map[string]bool{}
		}
		seen[s.Type][s.Locale] = true
		if strings.TrimSpace(s.Subject) == "" {
			t.Errorf("%s: empty subject", id)
		}
		if strings.TrimSpace(s.Preheader) == "" {
			t.Errorf("%s: empty preheader", id)
		}
		if strings.TrimSpace(s.Text) == "" {
			t.Errorf("%s: empty text part", id)
		}
		for _, part := range []string{s.Subject, s.HTML, s.Text} {
			if strings.Contains(part, "{{") || strings.Contains(part, "}}") {
				t.Errorf("%s: leftover placeholder", id)
				break
			}
		}
		dir := i18n.Dir(i18n.Normalize(s.Locale))
		if !strings.Contains(s.HTML, `<html lang="`) || !strings.Contains(s.HTML, `dir="`+dir+`"`) {
			t.Errorf("%s: html lacks lang/dir %s", id, dir)
		}
		if !strings.Contains(s.HTML, `name="color-scheme"`) || !strings.Contains(s.HTML, `role="presentation"`) {
			t.Errorf("%s: html lacks the shared frame", id)
		}
	}
	locales := Locales()
	// Every catalog event that has e-mail defaults, the migration seeded
	// account e-mails and the code-built e-mails, in all locales.
	want := []string{
		"auth.password_reset", "auth.email_verification", "auth.welcome", "notifications.test",
		"whatsapp.alarm.logged_out", "whatsapp.alarm.banned",
		"fleet.report.attachment", "fleet.report.link", "campaign.email",
	}
	for _, e := range catalog.All() {
		for _, tp := range e.Templates {
			if tp.Channel == catalog.ChannelEmail {
				want = append(want, e.Code)
				break
			}
		}
	}
	for _, typ := range want {
		for _, l := range locales {
			if !seen[typ][l] {
				t.Errorf("%s: no e-mail in %s", typ, l)
			}
		}
	}
	if len(want) < 30 {
		t.Fatalf("only %d e-mail types", len(want))
	}
}

// Catalog events whose default channels include e-mail have a template
// (catalog default or migration seed) to render.
func TestEmailEventsHaveTemplates(t *testing.T) {
	tpls, err := emailTemplates()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range catalog.All() {
		if slices.Contains(e.DefaultChannels, catalog.ChannelEmail) && len(tpls[e.Code]) == 0 {
			t.Errorf("%s: e-mail is a default channel but no e-mail template exists", e.Code)
		}
	}
}

func TestBuildRejectsUnknownLocale(t *testing.T) {
	if _, err := Build(context.Background(), []string{"xx"}); err == nil {
		t.Fatal("want error")
	}
}

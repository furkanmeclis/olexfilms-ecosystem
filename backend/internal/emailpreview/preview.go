// Package emailpreview renders every e-mail the backend sends with sample
// data, without a database or network (cmd/email-preview, TEC-1235): the
// notification templates (Go catalog defaults and the migration seeds),
// the WhatsApp connection alarm, the fleet report and the campaign e-mail.
package emailpreview

import (
	"context"
	"fmt"
	"html"
	"sort"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	campaignsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/campaigns/usecase"
	fleetmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/model"
	fleetusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/fleet/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/providers"
	whatsappusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/mail"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/migrations"
	"github.com/jackc/pgx/v5/pgtype"
)

// Sample groups.
const (
	GroupNotification = "notification" // notification_templates (catalog + migration seeds)
	GroupAlarm        = "whatsapp_alarm"
	GroupFleet        = "fleet_report"
	GroupCampaign     = "campaign"
)

// BaseURL is the sample web origin of the preview links.
const BaseURL = "https://app.olexfilms.example"

// Sample is one rendered e-mail.
type Sample struct {
	Group     string
	Type      string // template code or variant, e.g. TASK_ASSIGNED, fleet.report.link
	Source    string // "catalog", "migration" or "code"
	Locale    string
	Subject   string
	Preheader string
	HTML      string
	Text      string
	// Fallback is the template language when the locale has no template.
	Fallback string
}

// Locales returns the supported locales as strings.
func Locales() []string {
	out := make([]string, len(i18n.Supported))
	for i, l := range i18n.Supported {
		out[i] = string(l)
	}
	return out
}

// sampleBrand is the frame used by every preview.
var sampleBrand = providers.EmailBrand{Name: mail.DefaultBrandName, Color: mail.DefaultColor}

func brandFn(context.Context, int64) providers.EmailBrand { return sampleBrand }

// template is one stored e-mail template variant.
type template struct {
	language, subject, body, source string
}

// emailTemplates returns the generic e-mail templates per code: the
// migration seeds first (they exist before SyncCatalog runs, which never
// overwrites a row), then the Go catalog defaults for missing languages.
func emailTemplates() (map[string][]template, error) {
	seeds, err := migrationSeeds(migrations.FS())
	if err != nil {
		return nil, err
	}
	out := map[string][]template{}
	has := map[string]bool{}
	add := func(code, lang, subject, body, source string) {
		k := code + "|" + lang
		if has[k] {
			return
		}
		has[k] = true
		out[code] = append(out[code], template{language: lang, subject: subject, body: body, source: source})
	}
	for _, r := range seeds {
		if r.channel == catalog.ChannelEmail && r.role == catalog.RoleGeneric {
			add(r.code, r.language, r.subject, r.body, "migration")
		}
	}
	for _, e := range catalog.All() {
		for _, t := range e.Templates {
			if t.Channel == catalog.ChannelEmail && t.Role == catalog.RoleGeneric {
				add(e.Code, t.Language, t.Subject, t.Body, "catalog")
			}
		}
	}
	return out, nil
}

// pick returns the template for locale along the K10 chain (locale, en, tr).
func pick(ts []template, locale string) (template, bool) {
	variants := make([]msgtemplate.Variant, len(ts))
	for i, t := range ts {
		variants[i] = msgtemplate.Variant{Role: catalog.RoleGeneric, Language: t.language}
	}
	i, ok := msgtemplate.Pick(variants, catalog.RoleGeneric, msgtemplate.LocaleChain(locale, "", ""))
	if !ok {
		return template{}, false
	}
	return ts[i], true
}

// authSamples are the placeholders of the migration seeded auth templates.
var authSamples = []msgtemplate.Placeholder{
	{Key: "name", SampleTR: "Ahmet Yılmaz", SampleEN: "John Smith"},
	{Key: "code", SampleTR: "482913", SampleEN: "482913"},
	{Key: "expires_minutes", SampleTR: "15", SampleEN: "15"},
}

// vars are the sample values of a code in a locale.
func vars(code, locale string) map[string]string {
	out := map[string]string{}
	for _, p := range authSamples {
		out[p.Key] = p.Sample(locale)
	}
	if spec, ok := msgtemplate.Lookup(code); ok {
		for k, v := range spec.SampleVars(locale) {
			out[k] = v
		}
	}
	if e, ok := catalog.Lookup(code); ok {
		for k, v := range e.SampleVars(locale) {
			out[k] = v
		}
	}
	return out
}

// Build renders every e-mail for the locales (all supported when empty).
func Build(ctx context.Context, locales []string) ([]Sample, error) {
	if len(locales) == 0 {
		locales = Locales()
	}
	for i, l := range locales {
		loc, ok := i18n.Parse(l)
		if !ok {
			return nil, fmt.Errorf("unsupported locale %q", l)
		}
		locales[i] = string(loc)
	}
	var out []Sample
	n, err := notificationSamples(ctx, locales)
	if err != nil {
		return nil, err
	}
	out = append(out, n...)
	for _, f := range []func(context.Context, []string) ([]Sample, error){alarmSamples, fleetSamples, campaignSamples} {
		s, err := f(ctx, locales)
		if err != nil {
			return nil, err
		}
		out = append(out, s...)
	}
	return out, nil
}

func notificationProvider() providers.EmailProvider {
	return providers.EmailProvider{Brand: brandFn, BaseURL: BaseURL}
}

func render(ctx context.Context, p providers.EmailProvider, n db.Notification) (mail.Message, mail.Layout, error) {
	msg, err := p.Message(ctx, n)
	return msg, p.Layout(ctx, n), err
}

func notificationSamples(ctx context.Context, locales []string) ([]Sample, error) {
	tpls, err := emailTemplates()
	if err != nil {
		return nil, err
	}
	codes := make([]string, 0, len(tpls))
	for c := range tpls {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	p := notificationProvider()
	var out []Sample
	for _, code := range codes {
		for _, loc := range locales {
			t, ok := pick(tpls[code], loc)
			if !ok {
				continue
			}
			v := vars(code, loc)
			n := db.Notification{
				Title:    msgtemplate.RenderLocale(t.subject, v, loc),
				Body:     msgtemplate.RenderLocale(t.body, v, loc),
				Language: pgtype.Text{String: loc, Valid: true},
			}
			// Account e-mails carry a code, not a link; the rest open the panel.
			if !strings.HasPrefix(code, "auth.") {
				n.ActionUrl = pgtype.Text{String: "/notifications", Valid: true}
			}
			msg, l, err := render(ctx, p, n)
			if err != nil {
				return nil, fmt.Errorf("%s/%s: %w", code, loc, err)
			}
			s := Sample{
				Group: GroupNotification, Type: code, Source: t.source, Locale: loc,
				Subject: msg.Subject, Preheader: l.PreheaderText(), HTML: msg.HTMLBody, Text: msg.Body,
			}
			if t.language != loc {
				s.Fallback = t.language
			}
			out = append(out, s)
		}
	}
	return out, nil
}

func alarmSamples(ctx context.Context, locales []string) ([]Sample, error) {
	p := notificationProvider()
	var out []Sample
	for _, state := range []string{whatsapp.StateLoggedOut, whatsapp.StateBanned} {
		for _, loc := range locales {
			a := whatsappusecase.AlarmText(loc, state, state)
			n := db.Notification{
				Title: a.Title, Body: a.Body, Language: pgtype.Text{String: loc, Valid: true},
				ActionUrl: pgtype.Text{String: whatsappusecase.AlarmActionURL, Valid: true},
			}
			msg, l, err := render(ctx, p, n)
			if err != nil {
				return nil, err
			}
			s := Sample{
				Group: GroupAlarm, Type: "whatsapp.alarm." + state, Source: "code", Locale: loc,
				Subject: msg.Subject, Preheader: l.PreheaderText(), HTML: msg.HTMLBody, Text: msg.Body,
			}
			if a.Lang != loc {
				s.Fallback = a.Lang
			}
			out = append(out, s)
		}
	}
	return out, nil
}

func fleetSamples(ctx context.Context, locales []string) ([]Sample, error) {
	cfg := fleetusecase.ReportConfig{
		PortalURL: BaseURL + "/portal/fleet/reports",
		Brand: func(context.Context, int64) fleetusecase.MailBrand {
			return fleetusecase.MailBrand{Name: sampleBrand.Name, LogoURL: sampleBrand.LogoURL, Color: sampleBrand.Color}
		},
	}
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	f := db.Organization{Name: "Demo Lojistik"}
	var out []Sample
	for _, variant := range []string{"attachment", "link"} {
		var pdf []byte
		if variant == "attachment" {
			pdf = []byte("%PDF-1.7 preview")
		}
		for _, loc := range locales {
			r := db.FleetReport{
				PeriodKind: fleetmodel.ReportMonthly, Locale: loc,
				PeriodStart: pgtype.Date{Time: start, Valid: true},
				PeriodEnd:   pgtype.Date{Time: start.AddDate(0, 1, -1), Valid: true},
			}
			msg, err := fleetusecase.ReportMessage(ctx, cfg, f, r, pdf)
			if err != nil {
				return nil, err
			}
			out = append(out, Sample{
				Group: GroupFleet, Type: "fleet.report." + variant, Source: "code", Locale: loc,
				Subject: msg.Subject, Preheader: preheaderOf(msg.HTMLBody), HTML: msg.HTMLBody, Text: msg.Body,
			})
		}
	}
	return out, nil
}

// campaignContent is sample campaign content (written by the brand).
var campaignContent = map[string][2]string{
	"tr": {"Sonbahar bakım kampanyası", "Aracınız kışa hazır mı? **30 Kasım**'a kadar seramik kaplama bakımında %20 indirim."},
	"en": {"Autumn care campaign", "Is your car ready for winter? Get **20% off** ceramic coating care until **30 November**."},
	"ar": {"حملة العناية الخريفية", "هل سيارتك جاهزة للشتاء؟ احصل على **خصم 20%** على صيانة الطلاء الخزفي حتى **30 نوفمبر**."},
}

func campaignSamples(ctx context.Context, locales []string) ([]Sample, error) {
	e := campaignsusecase.MailEmail{Brand: brandFn}
	var out []Sample
	for _, loc := range locales {
		c, ok := campaignContent[loc]
		if !ok {
			c = campaignContent["en"]
		}
		msg, err := e.Message(ctx, campaignsusecase.EmailMessage{
			Locale: loc, Subject: c[0], Body: c[1],
			Deeplink:       BaseURL + "/campaigns/autumn",
			UnsubscribeURL: BaseURL + campaignsusecase.UnsubscribePath + "sample-token",
		})
		if err != nil {
			return nil, err
		}
		out = append(out, Sample{
			Group: GroupCampaign, Type: "campaign.email", Source: "code", Locale: loc,
			Subject: msg.Subject, Preheader: preheaderOf(msg.HTMLBody), HTML: msg.HTMLBody, Text: msg.Body,
		})
	}
	return out, nil
}

// preheaderOf reads the hidden preview text back from a rendered e-mail.
func preheaderOf(page string) string {
	const marker = `class="em-pre"`
	i := strings.Index(page, marker)
	if i < 0 {
		return ""
	}
	rest := page[i:]
	j := strings.IndexByte(rest, '>')
	k := strings.Index(rest, "&#847;")
	if j < 0 || k < j {
		return ""
	}
	return unescape(rest[j+1 : k])
}

func unescape(s string) string {
	return html.UnescapeString(s)
}

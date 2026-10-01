package msgtemplate

import (
	"sort"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
)

// Audience of a template type.
const (
	AudienceCustomer = "customer"
	AudienceStaff    = "staff"
)

// Channels.
const (
	ChannelInapp    = "inapp"
	ChannelEmail    = "email"
	ChannelWhatsApp = "whatsapp"
	ChannelSMS      = "sms"
)

// Locales a template may be written in (K10, i18n.Supported). Defaults are
// seeded for tr and en; a missing locale follows LocaleChain at send time.
var Locales = func() []string {
	out := make([]string, len(i18n.Supported))
	for i, l := range i18n.Supported {
		out[i] = string(l)
	}
	return out
}()

// Placeholder is one allowed `{{key}}` with localized sample values.
type Placeholder struct {
	Key      string `json:"key"`
	SampleTR string `json:"sample_tr"`
	SampleEN string `json:"sample_en"`
}

// Sample returns the sample for a locale: tr samples for tr, a localized
// sample (samples.go) when the locale has one, otherwise the en sample (the
// en fallback), tr when no en sample exists.
func (p Placeholder) Sample(locale string) string {
	l, ok := i18n.Parse(locale)
	if ok && l != i18n.LocaleTR && l != i18n.LocaleEN {
		if v, found := localizedSample(p.Key, l); found {
			return v
		}
	}
	if ok && l != i18n.LocaleTR && p.SampleEN != "" {
		return p.SampleEN
	}
	return p.SampleTR
}

// TypeSpec describes one message/notification template type.
type TypeSpec struct {
	Type         string        `json:"type"`
	Group        string        `json:"group"`
	Audience     string        `json:"audience"`
	Channels     []string      `json:"channels"`
	Placeholders []Placeholder `json:"placeholders"`
	// UserPreference marks staff types that appear in user notification preferences.
	UserPreference bool `json:"user_preference"`
}

// SampleVars returns preview values for a locale.
func (t TypeSpec) SampleVars(locale string) map[string]string {
	out := make(map[string]string, len(t.Placeholders))
	for _, p := range t.Placeholders {
		out[p.Key] = p.Sample(locale)
	}
	return out
}

// HasChannel reports whether the type supports a channel.
func (t TypeSpec) HasChannel(ch string) bool {
	for _, c := range t.Channels {
		if c == ch {
			return true
		}
	}
	return false
}

var (
	phCustomer = Placeholder{Key: "customer_name", SampleTR: "Ahmet Yılmaz", SampleEN: "John Smith"}
	phCompany  = Placeholder{Key: "company_name", SampleTR: "Tech Oto", SampleEN: "Tech Oto"}
	// business_name is the legacy alias of company_name.
	phBusiness = Placeholder{Key: "business_name", SampleTR: "Tech Oto", SampleEN: "Tech Oto"}
	phPlate    = Placeholder{Key: "plate", SampleTR: "34 ABC 123", SampleEN: "34 ABC 123"}
	phJob      = Placeholder{Key: "job_id", SampleTR: "JOB-DEMO-001", SampleEN: "JOB-DEMO-001"}
	phAmount   = Placeholder{Key: "amount", SampleTR: "1.250,00", SampleEN: "1,250.00"}
	phCurrency = Placeholder{Key: "currency", SampleTR: "TRY", SampleEN: "TRY"}
	phContract = Placeholder{Key: "contract_title", SampleTR: "Hizmet Sözleşmesi", SampleEN: "Service Agreement"}

	phQuoteNo    = Placeholder{Key: "quote_number", SampleTR: "TKL-000042", SampleEN: "QUO-000042"}
	phQuoteTotal = Placeholder{Key: "total_amount", SampleTR: "12.500,00 TRY", SampleEN: "TRY 12,500.00"}
	phValidUntil = Placeholder{Key: "valid_until", SampleTR: "30.09.2026", SampleEN: "Sep 30, 2026"}
	phQuoteLink  = Placeholder{Key: "quote_link", SampleTR: "https://olexfilms.app/q/abc123", SampleEN: "https://olexfilms.app/q/abc123"}
	phCreatedBy  = Placeholder{Key: "created_by_name", SampleTR: "Mehmet Kaya", SampleEN: "Mark Brown"}
	// app_link is the absolute in-app link of the notification (staff types).
	phAppLink = Placeholder{Key: "app_link", SampleTR: "https://olexfilms.app/t/tech-oto/quotes/abc", SampleEN: "https://olexfilms.app/t/tech-oto/quotes/abc"}

	phTodoTitle = Placeholder{Key: "todo_title", SampleTR: "34 ABC 123 seramik kontrolü", SampleEN: "Ceramic coating check 34 ABC 123"}
	phDueAt     = Placeholder{Key: "due_at", SampleTR: "25.09.2026 14:30", SampleEN: "Sep 25, 2026 14:30"}
	phDueIn     = Placeholder{Key: "due_in", SampleTR: "15 dakika sonra", SampleEN: "in 15 minutes"}
	phAssignee  = Placeholder{Key: "assignee_name", SampleTR: "Ayşe Demir", SampleEN: "Jane Doe"}
	phNotes     = Placeholder{Key: "todo_notes", SampleTR: "Müşteriyi arayın.", SampleEN: "Call the customer."}
	phTodoLink  = Placeholder{Key: "todo_link", SampleTR: "https://olexfilms.app/t/tech-oto/todos", SampleEN: "https://olexfilms.app/t/tech-oto/todos"}
)

var customerChannels = []string{ChannelWhatsApp, ChannelSMS}

var registry = map[string]TypeSpec{}
var order []string

// Register adds (or replaces) a template type. Call from init() in the module
// that owns the type, e.g. the quotes module for new quote.* types.
func Register(spec TypeSpec) {
	if _, ok := registry[spec.Type]; !ok {
		order = append(order, spec.Type)
	}
	registry[spec.Type] = spec
}

func init() {
	jobPh := []Placeholder{phCustomer, phCompany, phBusiness, phJob, phPlate}
	for _, t := range []string{"job.created", "job.ready", "job.delivered", "job.cancelled"} {
		Register(TypeSpec{Type: t, Group: "jobs", Audience: AudienceCustomer, Channels: customerChannels, Placeholders: jobPh})
	}
	Register(TypeSpec{Type: "job.paid", Group: "jobs", Audience: AudienceCustomer, Channels: customerChannels,
		Placeholders: []Placeholder{phCustomer, phCompany, phBusiness, phJob, phAmount, phCurrency, phPlate}})
	Register(TypeSpec{Type: "contract.signed", Group: "contracts", Audience: AudienceCustomer, Channels: customerChannels,
		Placeholders: []Placeholder{phCustomer, phCompany, phBusiness, phContract, phPlate, phJob}})
	Register(TypeSpec{Type: "sale.created", Group: "sales", Audience: AudienceCustomer, Channels: customerChannels,
		Placeholders: []Placeholder{phCustomer, phCompany, phBusiness, phAmount, phCurrency, phPlate}})

	quotePh := []Placeholder{phCustomer, phCompany, phQuoteNo, phQuoteTotal, phValidUntil, phQuoteLink, phPlate}
	for _, t := range []string{"quote.created", "quote.sent", "quote.reminder", "quote.expiring"} {
		Register(TypeSpec{Type: t, Group: "quotes", Audience: AudienceCustomer,
			Channels: []string{ChannelWhatsApp, ChannelSMS, ChannelEmail}, Placeholders: quotePh})
	}
	// Team-facing quote notifications (in-app / e-mail, per-user preferences).
	teamQuotePh := []Placeholder{phQuoteNo, phCustomer, phQuoteTotal, phValidUntil, phCreatedBy, phAssignee, phCompany, phAppLink}
	for _, t := range []string{"quote.team_created", "quote.team_expiring", "quote.team_accepted", "quote.team_rejected"} {
		Register(TypeSpec{Type: t, Group: "quotes", Audience: AudienceStaff,
			Channels: []string{ChannelInapp, ChannelEmail}, Placeholders: teamQuotePh, UserPreference: true})
	}
	Register(TypeSpec{Type: "todo.reminder", Group: "todos", Audience: AudienceStaff,
		Channels:       []string{ChannelInapp, ChannelEmail, ChannelWhatsApp, ChannelSMS},
		Placeholders:   []Placeholder{phTodoTitle, phDueAt, phDueIn, phAssignee, phNotes, phCustomer, phCompany, phTodoLink},
		UserPreference: true,
	})
}

// Lookup returns a registered type.
func Lookup(t string) (TypeSpec, bool) {
	s, ok := registry[t]
	return s, ok
}

// All returns registered types in registration order.
func All() []TypeSpec {
	out := make([]TypeSpec, 0, len(order))
	for _, t := range order {
		out = append(out, registry[t])
	}
	return out
}

// UserPreferenceTypes returns staff types that users configure per channel.
func UserPreferenceTypes() []TypeSpec {
	var out []TypeSpec
	for _, s := range All() {
		if s.UserPreference {
			out = append(out, s)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Type < out[j].Type })
	return out
}

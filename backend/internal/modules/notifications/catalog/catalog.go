// Package catalog is the notification event catalog: every event the
// notification center can send, its default channels, audience roles and
// allowed template placeholders. It is the source of truth; the
// notification_events table mirrors it (Sync at start-up) for the admin UI.
//
// Critical one-time passwords never pass through here: the OTP service sends
// them directly over the WhatsApp provider (TEC-92). The center carries
// regular notifications only.
package catalog

import (
	"sort"
	"sync"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
)

// Channels (six). realtime is not a channel: Centrifugo publish is the
// in-app channel's side effect.
const (
	ChannelInapp    = "inapp"
	ChannelEmail    = "email"
	ChannelWebPush  = "webpush"
	ChannelExpoPush = "expo_push"
	ChannelSMS      = "sms"
	ChannelWhatsApp = "whatsapp"
)

// Channels lists every channel in display order.
var Channels = []string{ChannelInapp, ChannelEmail, ChannelWebPush, ChannelExpoPush, ChannelSMS, ChannelWhatsApp}

// IsChannel reports whether ch is a known channel.
func IsChannel(ch string) bool {
	for _, c := range Channels {
		if c == ch {
			return true
		}
	}
	return false
}

// Template roles: the recipient's place in the organization tree. generic
// is the fallback written once for every role.
const (
	RoleGeneric     = "generic"
	RoleCustomer    = "customer"
	RoleDealer      = "dealer"
	RoleDistributor = "distributor"
	RoleCenter      = "center"
)

// Roles lists the template roles.
var Roles = []string{RoleGeneric, RoleCustomer, RoleDealer, RoleDistributor, RoleCenter}

// IsRole reports whether r is a template role.
func IsRole(r string) bool {
	for _, x := range Roles {
		if x == r {
			return true
		}
	}
	return false
}

// Event is one catalog entry.
type Event struct {
	Code   string `json:"code"`
	Module string `json:"module"`
	// DefaultChannels are used when the caller does not name channels.
	DefaultChannels []string `json:"default_channels"`
	// Critical events skip user preferences (admin channel switches still apply).
	Critical bool `json:"critical"`
	// AudienceRoles bounds role based fan-out (documentation + editor matrix).
	AudienceRoles []string                  `json:"audience_roles"`
	Placeholders  []msgtemplate.Placeholder `json:"placeholders"`
	// UserConfigurable events appear in the user preference matrix.
	UserConfigurable bool `json:"user_configurable"`
	// Templates are default rows inserted at start-up when missing (an
	// admin edit is never overwritten). Events seeded by migrations leave
	// it empty.
	Templates []DefaultTemplate `json:"-"`
}

// DefaultTemplate is one default notification_templates row.
type DefaultTemplate struct {
	Role     string
	Channel  string
	Language string
	Subject  string
	Body     string
	Format   string
}

// Spec returns the msgtemplate type spec used to validate placeholders.
func (e Event) Spec() msgtemplate.TypeSpec {
	return msgtemplate.TypeSpec{Type: e.Code, Group: e.Module, Channels: e.DefaultChannels, Placeholders: e.Placeholders}
}

// SampleVars returns preview values for a locale.
func (e Event) SampleVars(locale string) map[string]string {
	return e.Spec().SampleVars(locale)
}

var (
	mu     sync.RWMutex
	events = map[string]Event{}
)

// Register adds or replaces an event. Modules call it from init().
func Register(e Event) {
	mu.Lock()
	defer mu.Unlock()
	events[e.Code] = e
}

// Lookup returns a registered event.
func Lookup(code string) (Event, bool) {
	mu.RLock()
	defer mu.RUnlock()
	e, ok := events[code]
	return e, ok
}

// All returns the events sorted by code.
func All() []Event {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Event, 0, len(events))
	for _, e := range events {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

func ph(key, tr, en string) msgtemplate.Placeholder {
	return msgtemplate.Placeholder{Key: key, SampleTR: tr, SampleEN: en}
}

// Event codes registered by the platform.
const (
	EventNotificationsTest      = "notifications.test"
	EventFeaturesModuleRequest  = "features.module_requested"
	EventCustomersAssigned      = "customers.assigned"
	EventCustomersStatusBlocked = "customers.status_blocked"
	EventConversationsAssigned  = "conversations.assigned"
	EventAIPipelineEscalated    = "ai.pipeline.escalated"
	EventAIDraftPending         = "ai.draft.pending"
)

var staff = []string{RoleDealer, RoleDistributor, RoleCenter}

func init() {
	Register(Event{
		Code: EventNotificationsTest, Module: "notifications",
		DefaultChannels: []string{ChannelInapp},
		AudienceRoles:   Roles[1:],
		Placeholders:    []msgtemplate.Placeholder{ph("name", "Ahmet Yılmaz", "John Smith")},
	})
	Register(Event{
		Code: EventFeaturesModuleRequest, Module: "modules",
		DefaultChannels: []string{ChannelInapp, ChannelEmail},
		AudienceRoles:   []string{RoleDistributor, RoleCenter},
		Placeholders: []msgtemplate.Placeholder{
			ph("organization_name", "Tech Oto", "Tech Oto"),
			ph("module_key", "stock_forecast", "stock_forecast"),
			ph("note", "Lütfen açar mısınız?", "Please switch it on."),
		},
		UserConfigurable: true,
	})
	customer := ph("customer_name", "Ahmet Yılmaz", "John Smith")
	Register(Event{
		Code: EventCustomersAssigned, Module: "customers", DefaultChannels: []string{ChannelInapp},
		AudienceRoles: staff, Placeholders: []msgtemplate.Placeholder{customer}, UserConfigurable: true,
	})
	Register(Event{
		Code: EventCustomersStatusBlocked, Module: "customers", DefaultChannels: []string{ChannelInapp},
		AudienceRoles: staff, Placeholders: []msgtemplate.Placeholder{customer}, UserConfigurable: true,
	})
	Register(Event{
		Code: EventConversationsAssigned, Module: "conversations", DefaultChannels: []string{ChannelInapp},
		AudienceRoles: staff, UserConfigurable: true,
	})
	Register(Event{
		Code: EventAIPipelineEscalated, Module: "ai", DefaultChannels: []string{ChannelInapp},
		AudienceRoles: staff, Placeholders: []msgtemplate.Placeholder{ph("intent", "fiyat", "pricing")},
		UserConfigurable: true,
	})
	Register(Event{
		Code: EventAIDraftPending, Module: "ai", DefaultChannels: []string{ChannelInapp},
		AudienceRoles: staff, UserConfigurable: true,
	})
}

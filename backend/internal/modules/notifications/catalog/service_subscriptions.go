package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

const (
	EventServiceSubscriptionAssigned        = "SERVICE_SUBSCRIPTION_ASSIGNED"
	EventServiceSubscriptionCancelRequested = "SERVICE_SUBSCRIPTION_CANCEL_REQUESTED"
	EventServiceSubscriptionCancelled       = "SERVICE_SUBSCRIPTION_CANCELLED"
	EventServiceSubscriptionCancelRejected  = "SERVICE_SUBSCRIPTION_CANCEL_REJECTED"
	// TEC-308: expiry and the 30 / 7 day reminder.
	EventServiceSubscriptionExpired  = "SERVICE_SUBSCRIPTION_EXPIRED"
	EventServiceSubscriptionExpiring = "SERVICE_SUBSCRIPTION_EXPIRING"
)

var ServiceSubscriptionChannels = []string{ChannelInapp, ChannelEmail}

var serviceSubscriptionEvents = []string{
	EventServiceSubscriptionAssigned,
	EventServiceSubscriptionCancelRequested,
	EventServiceSubscriptionCancelled,
	EventServiceSubscriptionCancelRejected,
	EventServiceSubscriptionExpired,
	EventServiceSubscriptionExpiring,
}

var serviceSubscriptionTexts = map[string]map[string]localizedText{
	EventServiceSubscriptionAssigned: {
		"tr": {"Hizmet aboneligi atandi: {{item_name}}", "{{item_name}} hizmeti {{organization_name}} icin baslatildi. Bitis tarihi: {{ends_on}}."},
		"en": {"Service subscription assigned: {{item_name}}", "{{item_name}} was assigned to {{organization_name}}. End date: {{ends_on}}."},
	},
	EventServiceSubscriptionCancelRequested: {
		"tr": {"Erken iptal talebi: {{item_name}}", "{{organization_name}}, {{item_name}} aboneligi icin erken iptal talep etti. Gerekce: {{reason}}"},
		"en": {"Early cancellation requested: {{item_name}}", "{{organization_name}} requested early cancellation for {{item_name}}. Reason: {{reason}}"},
	},
	EventServiceSubscriptionCancelled: {
		"tr": {"Abonelik iptal edildi: {{item_name}}", "{{item_name}} aboneligi iptal edildi. Cayma bedeli: {{cancellation_fee}} {{currency}}."},
		"en": {"Subscription cancelled: {{item_name}}", "{{item_name}} was cancelled. Cancellation fee: {{cancellation_fee}} {{currency}}."},
	},
	EventServiceSubscriptionCancelRejected: {
		"tr": {"Iptal talebi reddedildi: {{item_name}}", "{{item_name}} aboneligi icin erken iptal talebi reddedildi."},
		"en": {"Cancellation request rejected: {{item_name}}", "The early cancellation request for {{item_name}} was rejected."},
	},
	EventServiceSubscriptionExpired: {
		"tr": {"Abonelik sona erdi: {{item_name}}", "{{organization_name}} icin {{item_name}} aboneligi {{ends_on}} tarihinde sona erdi."},
		"en": {"Subscription ended: {{item_name}}", "The {{item_name}} subscription of {{organization_name}} ended on {{ends_on}}."},
	},
	EventServiceSubscriptionExpiring: {
		"tr": {"Abonelik {{days_before}} gun icinde bitiyor: {{item_name}}", "{{organization_name}} icin {{item_name}} aboneligi {{ends_on}} tarihinde sona erecek."},
		"en": {"Subscription ends within {{days_before}} days: {{item_name}}", "The {{item_name}} subscription of {{organization_name}} ends on {{ends_on}}."},
	},
}

func serviceSubscriptionTemplates(code string) []DefaultTemplate {
	texts := completeServiceSubscriptionTexts(serviceSubscriptionTexts[code])
	out := make([]DefaultTemplate, 0, len(texts)*len(ServiceSubscriptionChannels))
	for _, lang := range msgtemplate.Locales {
		t, ok := texts[lang]
		if !ok {
			continue
		}
		for _, ch := range ServiceSubscriptionChannels {
			out = append(out, DefaultTemplate{
				Role: RoleGeneric, Channel: ch, Language: lang,
				Subject: t.subject, Body: t.body, Format: "text",
			})
		}
	}
	return out
}

func completeServiceSubscriptionTexts(base map[string]localizedText) map[string]localizedText {
	out := make(map[string]localizedText, len(msgtemplate.Locales))
	en := base["en"]
	for _, lang := range msgtemplate.Locales {
		out[lang] = en
	}
	for lang, text := range base {
		out[lang] = text
	}
	return out
}

func serviceSubscriptionPlaceholders() []msgtemplate.Placeholder {
	return []msgtemplate.Placeholder{
		ph("item_name", "Egitim paketi", "Training package"),
		ph("organization_name", "Tech Oto", "Tech Oto"),
		ph("ends_on", "2027-10-04", "2027-10-04"),
		ph("reason", "Kullanilmiyor", "No longer used"),
		ph("cancellation_fee", "100.00", "100.00"),
		ph("currency", "EUR", "EUR"),
		ph("days_before", "30", "30"),
	}
}

func init() {
	for _, code := range serviceSubscriptionEvents {
		Register(Event{
			Code: code, Module: "service_subscriptions",
			DefaultChannels:  ServiceSubscriptionChannels,
			AudienceRoles:    []string{RoleDealer, RoleDistributor, RoleCenter},
			Placeholders:     serviceSubscriptionPlaceholders(),
			UserConfigurable: true,
			Templates:        serviceSubscriptionTemplates(code),
		})
	}
}

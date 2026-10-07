package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

// WhatsApp inbox notifications (TEC-398, F4-02f). A new inbound message of
// an assigned conversation tells the assignee in-app and over web push
// (whatsapp.message.received with assigned_user_id). Per the F4 decision S2
// only platform admins read conversations, so the audience is the center.
// Default templates ship in all 13 locales (SyncCatalog, no migration).
const EventConversationInbound = "conversation.inbound"

// ConversationInboundChannels are the default channels of the inbound
// notification.
var ConversationInboundChannels = []string{ChannelInapp, ChannelWebPush}

var conversationInboundTexts = map[string]localizedText{
	"tr":    {"Yeni WhatsApp mesajı: {{contact_name}}", "{{contact_name}}: {{preview}}"},
	"en":    {"New WhatsApp message: {{contact_name}}", "{{contact_name}}: {{preview}}"},
	"bg":    {"Ново съобщение в WhatsApp: {{contact_name}}", "{{contact_name}}: {{preview}}"},
	"de":    {"Neue WhatsApp-Nachricht: {{contact_name}}", "{{contact_name}}: {{preview}}"},
	"el":    {"Νέο μήνυμα WhatsApp: {{contact_name}}", "{{contact_name}}: {{preview}}"},
	"uk":    {"Нове повідомлення WhatsApp: {{contact_name}}", "{{contact_name}}: {{preview}}"},
	"ru":    {"Новое сообщение WhatsApp: {{contact_name}}", "{{contact_name}}: {{preview}}"},
	"fr":    {"Nouveau message WhatsApp : {{contact_name}}", "{{contact_name}} : {{preview}}"},
	"es":    {"Nuevo mensaje de WhatsApp: {{contact_name}}", "{{contact_name}}: {{preview}}"},
	"it":    {"Nuovo messaggio WhatsApp: {{contact_name}}", "{{contact_name}}: {{preview}}"},
	"zh-CN": {"新的 WhatsApp 消息：{{contact_name}}", "{{contact_name}}：{{preview}}"},
	"az":    {"Yeni WhatsApp mesajı: {{contact_name}}", "{{contact_name}}: {{preview}}"},
	"ar":    {"رسالة واتساب جديدة: {{contact_name}}", "{{contact_name}}: {{preview}}"},
}

func conversationInboundTemplates() []DefaultTemplate {
	out := make([]DefaultTemplate, 0, len(conversationInboundTexts)*len(ConversationInboundChannels))
	for _, lang := range msgtemplate.Locales {
		t, ok := conversationInboundTexts[lang]
		if !ok {
			continue // TestConversationInboundTemplatesCoverEveryLocale reports it
		}
		for _, ch := range ConversationInboundChannels {
			out = append(out, DefaultTemplate{
				Role: RoleGeneric, Channel: ch, Language: lang,
				Subject: t.subject, Body: t.body, Format: "text",
			})
		}
	}
	return out
}

func init() {
	Register(Event{
		Code: EventConversationInbound, Module: "conversations",
		DefaultChannels: ConversationInboundChannels,
		AudienceRoles:   []string{RoleCenter},
		Placeholders: []msgtemplate.Placeholder{
			ph("contact_name", "Ahmet Yılmaz", "John Smith"),
			ph("preview", "Merhaba, randevu alabilir miyim?", "Hello, can I book an appointment?"),
		},
		UserConfigurable: true,
		Templates:        conversationInboundTemplates(),
	})
}

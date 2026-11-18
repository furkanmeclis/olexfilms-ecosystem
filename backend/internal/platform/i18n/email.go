package i18n

// EmailKeys are the TEC-1235 e-mail texts: the frame (button, fallback
// link, footer, unsubscribe; mail.LabelsFor) and the e-mails whose text the
// code owns (WhatsApp connection alarm, fleet report). They live in every
// locale catalog (TestEmailTextsCoverEveryLocale). Notification template
// bodies stay in notification_templates (admin editable).
var EmailKeys = []string{
	"email.button.open",
	"email.link_fallback",
	"email.footer.reason",
	"email.footer.reason_campaign",
	"email.campaign.details",
	"email.unsubscribe.intro",
	"email.unsubscribe.label",
	"email.whatsapp_alarm.logged_out.title",
	"email.whatsapp_alarm.logged_out.body",
	"email.whatsapp_alarm.logged_out.preheader",
	"email.whatsapp_alarm.banned.title",
	"email.whatsapp_alarm.banned.body",
	"email.whatsapp_alarm.banned.preheader",
	"email.fleet_report.subject",
	"email.fleet_report.body_attached",
	"email.fleet_report.body_link",
	"email.fleet_report.button",
	"email.fleet_report.preheader_attached",
	"email.fleet_report.preheader_link",
}

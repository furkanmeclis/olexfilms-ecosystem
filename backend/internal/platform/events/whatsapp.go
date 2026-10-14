package events

// WhatsApp gateway events (TEC-92). The F4 AI pipeline consumes
// whatsapp.message.received through the outbox.
const (
	WhatsAppMessageReceived = "whatsapp.message.received"
	WhatsAppConnectionAlarm = "whatsapp.connection.alarm"
	// WhatsAppMessageFailed: an outgoing message failed for good (TEC-635).
	// Emitted for campaign messages; payload campaign_recipient_id, reason.
	WhatsAppMessageFailed = "whatsapp.message.failed"
)

package events

// WhatsApp gateway events (TEC-92). The F4 AI pipeline consumes
// whatsapp.message.received through the outbox.
const (
	WhatsAppMessageReceived = "whatsapp.message.received"
	WhatsAppConnectionAlarm = "whatsapp.connection.alarm"
)

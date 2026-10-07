// Package model holds the AI assistant enums shared by the repository and
// the later usecase, handler, WhatsApp and MCP layers (TEC-383, F4-01a).
// Values mirror the CHECK constraints of migration 000101.
package model

// Conversation channels (ai_conversations.channel).
const (
	ChannelPanel  = "panel"
	ChannelPortal = "portal"
)

// Usage channels (ai_usage.channel): the conversation channels plus the
// non-chat callers.
const (
	UsageChannelPanel    = "panel"
	UsageChannelPortal   = "portal"
	UsageChannelWhatsApp = "whatsapp"
	UsageChannelMCP      = "mcp"
	UsageChannelTriage   = "triage"
)

// UsageChannels lists every ai_usage.channel value.
var UsageChannels = []string{
	UsageChannelPanel, UsageChannelPortal, UsageChannelWhatsApp, UsageChannelMCP, UsageChannelTriage,
}

// Usage purposes (ai_usage.purpose).
const (
	PurposeChat   = "chat"
	PurposeTitle  = "title"
	PurposeTriage = "triage"
	PurposeLocale = "locale"
)

// UsagePurposes lists every ai_usage.purpose value.
var UsagePurposes = []string{PurposeChat, PurposeTitle, PurposeTriage, PurposeLocale}

// Quota pools. PoolSystem is the brand center's separate pool for customer,
// visitor and triage calls; PoolOrg is the organization's own quota.
const (
	PoolOrg    = "org"
	PoolSystem = "system"
)

// UsagePools lists every pool value.
var UsagePools = []string{PoolOrg, PoolSystem}

// Message roles and statuses (ai_messages).
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"

	MessagePending   = "pending"
	MessageComplete  = "complete"
	MessageError     = "error"
	MessageCancelled = "cancelled"
)

// Pending action sources and statuses (ai_pending_actions).
const (
	SourcePanel    = "panel"
	SourcePortal   = "portal"
	SourceWhatsApp = "whatsapp"
	SourceMCP      = "mcp"

	ActionPending   = "pending"
	ActionExecuting = "executing"
	ActionConfirmed = "confirmed"
	ActionFailed    = "failed"
	ActionCancelled = "cancelled"
	ActionExpired   = "expired"
)

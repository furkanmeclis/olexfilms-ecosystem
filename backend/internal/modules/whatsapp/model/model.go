// Package model holds the WhatsApp conversation vocabulary (TEC-393,
// F4-02a). The values mirror the CHECK constraints of migration 000103.
package model

// Conversation inbox status.
const (
	StatusOpen    = "open"
	StatusPending = "pending"
	StatusClosed  = "closed"
)

// Statuses lists every conversation status (list filter whitelist).
var Statuses = []string{StatusOpen, StatusPending, StatusClosed}

// AI mode of a conversation: auto answers, paused until ai_paused_until (or
// until changed), off never answers.
const (
	AIModeAuto   = "auto"
	AIModePaused = "paused"
	AIModeOff    = "off"
)

// AIModes lists every AI mode (list filter whitelist).
var AIModes = []string{AIModeAuto, AIModePaused, AIModeOff}

// Resolved identity of the contact.
const (
	IdentityPanelUser = "panel_user"
	IdentityCustomer  = "customer"
	IdentityVisitor   = "visitor"
	IdentityUnknown   = "unknown"
)

// IdentityKinds lists every identity kind (list filter whitelist).
var IdentityKinds = []string{IdentityPanelUser, IdentityCustomer, IdentityVisitor, IdentityUnknown}

// Message sender types.
const (
	SenderContact = "contact"
	SenderStaff   = "staff"
	SenderSystem  = "system"
	SenderAI      = "ai"
)

// AI run status.
const (
	RunRunning   = "running"
	RunCompleted = "completed"
	RunFailed    = "failed"
	RunSkipped   = "skipped"
)

// AIRunRetentionDays is how long conversation_ai_runs rows are kept
// (QUESTIONS #15); messages are kept.
const AIRunRetentionDays = 90

// Opt-out scopes, actions and sources.
const (
	OptOutScopeMarketing = "marketing"
	OptOutScopeAI        = "ai"

	OptOutActionOut = "out"
	OptOutActionIn  = "in"

	OptOutSourceWhatsApp = "whatsapp"
	OptOutSourcePanel    = "panel"
	OptOutSourcePortal   = "portal"
	OptOutSourceCampaign = "campaign"
	OptOutSourceImport   = "import"
	OptOutSourceSystem   = "system"
)

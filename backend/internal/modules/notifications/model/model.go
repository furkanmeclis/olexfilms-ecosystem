package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	ChannelInapp    = "inapp"
	ChannelEmail    = "email"
	ChannelWebPush  = "webpush"
	ChannelExpoPush = "expo_push"
	ChannelSMS      = "sms"
	ChannelWhatsApp = "whatsapp"

	StatusQueued     = "queued"
	StatusProcessing = "processing"
	StatusSent       = "sent"
	StatusDelivered  = "delivered"
	StatusRead       = "read"
	StatusFailed     = "failed"
	StatusCancelled  = "cancelled"

	// Delivery statuses (notification_deliveries.status) besides the
	// notification ones above.
	DeliverySkippedDisabled    = "skipped_disabled"
	DeliverySkippedPreference  = "skipped_preference"
	DeliverySkippedNoTemplate  = "skipped_no_template"
	DeliverySkippedNoRecipient = "skipped_no_recipient"

	PriorityLow      = "low"
	PriorityNormal   = "normal"
	PriorityHigh     = "high"
	PriorityCritical = "critical"
)

// NotificationStatuses are the notifications.status values
// (notifications_status_chk), for list filter validation.
var NotificationStatuses = []string{
	StatusQueued, StatusProcessing, StatusSent, StatusDelivered, StatusRead, StatusFailed, StatusCancelled,
}

// Notification is the API projection.
type Notification struct {
	UUID            uuid.UUID         `json:"uuid"`
	Channel         string            `json:"channel"`
	Status          string            `json:"status"`
	Priority        string            `json:"priority"`
	Title           string            `json:"title"`
	Body            string            `json:"body"`
	Payload         map[string]any    `json:"payload"`
	ActionURL       *string           `json:"action_url,omitempty"`
	SignedActionURL *string           `json:"signed_action_url,omitempty"`
	Recipient       *string           `json:"recipient,omitempty"`
	TemplateCode    *string           `json:"template_code,omitempty"`
	SourceEvent     *string           `json:"source_event,omitempty"`
	SentAt          *time.Time        `json:"sent_at,omitempty"`
	ReadAt          *time.Time        `json:"read_at,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	User            *NotificationUser `json:"user,omitempty"`
}

// NotificationUser is the target user on a platform notification row.
type NotificationUser struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// Preferences is the user notification preference set. The bool fields are
// the global (every event) rows of the legacy channels; Rules holds every
// row, event specific ones included. realtime_enabled mirrors inapp
// (Centrifugo is part of the in-app channel) and is ignored on write.
type Preferences struct {
	EmailEnabled    bool             `json:"email_enabled"`
	InappEnabled    bool             `json:"inapp_enabled"`
	RealtimeEnabled bool             `json:"realtime_enabled"`
	PushEnabled     bool             `json:"push_enabled"`
	Rules           []PreferenceRule `json:"rules"`
}

// PreferenceRule is one preference row: EventCode nil = every event.
type PreferenceRule struct {
	EventCode *string `json:"event_code"`
	Channel   string  `json:"channel"`
	Enabled   bool    `json:"enabled"`
}

// EnqueueInput creates one or more queued notifications (legacy, template
// code based path used by auth/exports/imports/bulk). New code dispatches a
// catalog event with DispatchInput.
type EnqueueInput struct {
	TenantID     *int64
	WorkspaceID  *int64
	UserID       *int64
	UserUUID     *uuid.UUID
	Channels     []string
	Priority     string
	Title        string
	Body         string
	Payload      map[string]any
	ActionURL    *string
	Recipient    *string
	TemplateCode string
	TemplateVars map[string]string
	SourceEvent  string
	Language     string
	// SecurityEmail bypasses email preference (password reset / verification).
	SecurityEmail bool
}

// DispatchInput sends one catalog event to explicit recipients. EventID is
// the idempotency key together with (user, channel): dispatching the same
// event twice creates no new delivery.
type DispatchInput struct {
	EventID        uuid.UUID
	EventCode      string
	OrganizationID *int64
	BrandID        *int64
	// UserIDs are the recipients (resolved by the caller).
	UserIDs []int64
	// Channels overrides the catalog default channels when set.
	Channels  []string
	Vars      map[string]string
	Payload   map[string]any
	ActionURL *string
	Priority  string
}

// DispatchResult counts the deliveries Dispatch wrote.
type DispatchResult struct {
	Queued     int `json:"queued"`
	Skipped    int `json:"skipped"`
	Duplicates int `json:"duplicates"`
}

// Delivery is one notification_deliveries row (admin log).
type Delivery struct {
	UUID        uuid.UUID `json:"uuid"`
	EventID     uuid.UUID `json:"event_id"`
	EventCode   string    `json:"event_code"`
	UserUUID    uuid.UUID `json:"user_uuid"`
	UserEmail   string    `json:"user_email"`
	Channel     string    `json:"channel"`
	Role        string    `json:"role"`
	Language    string    `json:"language"`
	Status      string    `json:"status"`
	Provider    string    `json:"provider"`
	ProviderRef string    `json:"provider_ref"`
	Error       string    `json:"error"`
	Attempts    int32     `json:"attempts"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Template is one notification_templates row.
type Template struct {
	UUID      uuid.UUID `json:"uuid"`
	Code      string    `json:"code"`
	Role      string    `json:"role"`
	Channel   string    `json:"channel"`
	Language  string    `json:"language"`
	BrandID   *int64    `json:"brand_id"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	Format    string    `json:"format"`
	Active    bool      `json:"active"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TemplateInput upserts a template (event x role x channel x language).
type TemplateInput struct {
	Code     string `json:"code"`
	Role     string `json:"role"`
	Channel  string `json:"channel"`
	Language string `json:"language"`
	Subject  string `json:"subject"`
	Body     string `json:"body"`
	Format   string `json:"format"`
	Active   *bool  `json:"active"`
}

// Rendered is a template preview.
type Rendered struct {
	Subject  string `json:"subject"`
	Body     string `json:"body"`
	HTML     string `json:"html,omitempty"`
	Language string `json:"language"`
	Dir      string `json:"dir"`
}

// ChannelSetting is an admin channel switch.
type ChannelSetting struct {
	Channel   string    `json:"channel"`
	Enabled   bool      `json:"enabled"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PushDevice is a registered Expo push token.
type PushDevice struct {
	UUID       uuid.UUID `json:"uuid"`
	DeviceID   string    `json:"device_id"`
	Platform   string    `json:"platform"`
	AppVersion string    `json:"app_version,omitempty"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

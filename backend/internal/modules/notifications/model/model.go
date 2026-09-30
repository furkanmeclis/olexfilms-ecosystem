package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	ChannelInapp    = "inapp"
	ChannelEmail    = "email"
	ChannelRealtime = "realtime"
	ChannelSMS      = "sms"
	ChannelPush     = "push"

	StatusQueued     = "queued"
	StatusProcessing = "processing"
	StatusSent       = "sent"
	StatusDelivered  = "delivered"
	StatusRead       = "read"
	StatusFailed     = "failed"
	StatusCancelled  = "cancelled"

	PriorityLow      = "low"
	PriorityNormal   = "normal"
	PriorityHigh     = "high"
	PriorityCritical = "critical"
)

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

// Preferences is the user notification preference set.
type Preferences struct {
	EmailEnabled    bool `json:"email_enabled"`
	InappEnabled    bool `json:"inapp_enabled"`
	RealtimeEnabled bool `json:"realtime_enabled"`
	PushEnabled     bool `json:"push_enabled"`
}

// EnqueueInput creates one or more queued notifications.
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

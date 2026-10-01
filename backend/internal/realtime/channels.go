package realtime

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

const ChannelSystemNotifications = "system.notifications"

// ChannelAuthorizer decides whether a principal may subscribe to a channel.
type ChannelAuthorizer interface {
	CanSubscribeChannel(ctx context.Context, userUUID uuid.UUID, isSuperAdmin bool, channel string) (bool, error)
}

// UserChannel is the personal channel "user:{uuid}" (notifications).
func UserChannel(id uuid.UUID) string { return "user:" + id.String() }

// ParseUserChannel returns the user UUID from "user:{uuid}".
func ParseUserChannel(channel string) (uuid.UUID, bool) {
	raw, ok := stripPrefix(channel, "user:")
	if !ok {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// ParseWorkspaceChannel returns the workspace UUID from "workspace:{uuid}".
func ParseWorkspaceChannel(channel string) (uuid.UUID, bool) {
	raw, ok := stripPrefix(channel, "workspace:")
	if !ok {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// ParseConversationChannel returns the conversation UUID from "conversation:{uuid}".
func ParseConversationChannel(channel string) (uuid.UUID, bool) {
	raw, ok := stripPrefix(channel, "conversation:")
	if !ok {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

func stripPrefix(channel, prefix string) (string, bool) {
	channel = strings.TrimSpace(channel)
	if !strings.HasPrefix(channel, prefix) {
		return "", false
	}
	raw := strings.TrimPrefix(channel, prefix)
	if raw == "" {
		return "", false
	}
	return raw, true
}

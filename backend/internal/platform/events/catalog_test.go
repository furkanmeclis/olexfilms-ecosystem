package events

import (
	"testing"
)

func TestValidateEventName(t *testing.T) {
	t.Parallel()
	if err := ValidateEventName("customers.created"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateEventName("auth.password_reset"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateEventName("Bad.Name"); err == nil {
		t.Fatal("expected uppercase error")
	}
	if err := ValidateEventName("nodot"); err == nil {
		t.Fatal("expected missing-dot error")
	}
}

func TestAllKnownEvents_UniqueValidAndADRAligned(t *testing.T) {
	t.Parallel()
	seen := map[string]struct{}{}
	for _, name := range AllKnownEvents() {
		if _, ok := seen[name]; ok {
			t.Fatalf("duplicate known event %q", name)
		}
		seen[name] = struct{}{}
		if err := ValidateEventName(name); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}

	required := []string{
		CustomersCreated,
		CustomersUpdated,
		CustomersDeleted,
		CustomersRestored,
		CustomersStatusChanged,
		CustomersContactAdded,
		CustomersContactVerified,
		CustomersContactRemoved,
		CustomersWorkspaceAdded,
		CustomersWorkspaceRemoved,
		CustomersAssigned,
		CustomersUnassigned,
		CustomersTransferred,
		CustomersFlagAdded,
		CustomersFlagRemoved,
		CustomersNoteAdded,
		CustomersNoteRemoved,
		CustomersExternalAccountLinked,
		CustomersExternalAccountUnlinked,
	}
	for _, name := range required {
		if !IsKnownEvent(name) {
			t.Fatalf("expected ADR event known: %s", name)
		}
	}

	inboxRequired := []string{
		ConversationsCreated,
		ConversationsUpdated,
		ConversationsAssigned,
		ConversationsResolved,
		ConversationsReopened,
		MessagesReceived,
		MessagesSent,
		MessagesStatusChanged,
		ChannelAccountsConnected,
		ChannelAccountsDisconnected,
		ChannelAccountsError,
	}
	for _, name := range inboxRequired {
		if !IsKnownEvent(name) {
			t.Fatalf("expected inbox ADR event known: %s", name)
		}
	}

	aiRequired := []string{
		AIProfilesCreated,
		AIProfilesUpdated,
		AIProfilesDeleted,
		AIProfilesWorkspaceAdded,
		AIProfilesWorkspaceRemoved,
		AIProfilesChannelAttached,
		AIProfilesChannelDetached,
		AICredentialsCreated,
		AICredentialsUpdated,
		AICredentialsDeleted,
		AIPipelineCompleted,
		AIPipelineEscalated,
		AIPipelineGuardFailed,
		AIDraftCreated,
		AIConversationTakeover,
		AIConversationReleased,
	}
	for _, name := range aiRequired {
		if !IsKnownEvent(name) {
			t.Fatalf("expected AI ADR event known: %s", name)
		}
	}

	// ADR: archive is a flag, not customers.archived.
	if IsKnownEvent("customers.archived") {
		t.Fatal("customers.archived must not be in catalog; use flag_added/flag_removed")
	}
}

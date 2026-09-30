package events

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// NamingStandard is the canonical event name pattern: {module}.{action}.
const NamingStandard = `{module}.{action}` // e.g. customers.created, auth.password_reset

// Cari (accounts receivable) domain events.
const (
	CariChargePosted  = "cari.charge_posted"
	CariPaymentPosted = "cari.payment_posted"
	CariEntryVoided   = "cari.entry_voided"
)

// Jobs (operations / service jobs) domain events.
const (
	JobsCreated   = "jobs.created"
	JobsReady     = "jobs.ready"
	JobsDelivered = "jobs.delivered"
	JobsClosed    = "jobs.closed"
	JobsCancelled = "jobs.cancelled"
	JobsVoided    = "jobs.voided"
)

// Sales (quick product sales) domain events.
const (
	SalesCreated = "sales.created"
	SalesVoided  = "sales.voided"
)

// Suppliers domain events.
const (
	SuppliersCreated = "suppliers.created"
	SuppliersUpdated = "suppliers.updated"
	SuppliersDeleted = "suppliers.deleted"
)

// Purchases (stock purchase) domain events.
const (
	PurchasesCreated = "purchases.created"
	PurchasesVoided  = "purchases.voided"
)

// Customer domain events (ADR catalog v1).
// Archive is the archived flag → customers.flag_added / flag_removed (no customers.archived).
const (
	CustomersCreated                 = "customers.created"
	CustomersUpdated                 = "customers.updated"
	CustomersDeleted                 = "customers.deleted"
	CustomersRestored                = "customers.restored"
	CustomersStatusChanged           = "customers.status_changed"
	CustomersContactAdded            = "customers.contact_added"
	CustomersContactVerified         = "customers.contact_verified"
	CustomersContactRemoved          = "customers.contact_removed"
	CustomersWorkspaceAdded          = "customers.workspace_added"
	CustomersWorkspaceRemoved        = "customers.workspace_removed"
	CustomersAssigned                = "customers.assigned"
	CustomersUnassigned              = "customers.unassigned"
	CustomersTransferred             = "customers.transferred"
	CustomersFlagAdded               = "customers.flag_added"
	CustomersFlagRemoved             = "customers.flag_removed"
	CustomersNoteAdded               = "customers.note_added"
	CustomersNoteRemoved             = "customers.note_removed"
	CustomersExternalAccountLinked   = "customers.external_account_linked"
	CustomersExternalAccountUnlinked = "customers.external_account_unlinked"
)

// Inbox domain events (INBOX_DOMAIN_ADR catalog).
const (
	ConversationsCreated        = "conversations.created"
	ConversationsUpdated        = "conversations.updated"
	ConversationsAssigned       = "conversations.assigned"
	ConversationsResolved       = "conversations.resolved"
	ConversationsReopened       = "conversations.reopened"
	MessagesReceived            = "messages.received"
	MessagesSent                = "messages.sent"
	MessagesStatusChanged       = "messages.status_changed"
	ConversationsTyping         = "conversations.typing"
	ChannelAccountsConnected    = "channel_accounts.connected"
	ChannelAccountsDisconnected = "channel_accounts.disconnected"
	ChannelAccountsError        = "channel_accounts.error"
)

// Commerce domain events (COMMERCE_DOMAIN_ADR catalog).
const (
	CommerceAppsUpdated                 = "commerce.apps.updated"
	CommerceConnectionsCreated          = "commerce.connections.created"
	CommerceConnectionsUpdated          = "commerce.connections.updated"
	CommerceConnectionsDisconnected     = "commerce.connections.disconnected"
	CommerceConnectionsWorkspaceAdded   = "commerce.connections.workspace_added"
	CommerceConnectionsWorkspaceRemoved = "commerce.connections.workspace_removed"
	CommerceOrdersUpserted              = "commerce.orders.upserted"
	CommerceOrdersLinkedToCustomer      = "commerce.orders.linked_to_customer"
	CommerceRefundsUpdated              = "commerce.refunds.updated"
	CommerceCategoriesUpserted          = "commerce.categories.upserted"
	CommerceCategoriesDeleted           = "commerce.categories.deleted"
	CommerceProductsUpserted            = "commerce.products.upserted"
	CommerceProductsDeleted             = "commerce.products.deleted"
	CommerceCatalogSyncCompleted        = "commerce.catalog.sync_completed"
	CommerceCatalogSyncFailed           = "commerce.catalog.sync_failed"
)

// AI domain events (AI_DOMAIN_ADR catalog). Names only in 022; publish/handlers in 023+.
const (
	AIProfilesCreated          = "ai.profiles.created"
	AIProfilesUpdated          = "ai.profiles.updated"
	AIProfilesDeleted          = "ai.profiles.deleted"
	AIProfilesWorkspaceAdded   = "ai.profiles.workspace_added"
	AIProfilesWorkspaceRemoved = "ai.profiles.workspace_removed"
	AIProfilesChannelAttached  = "ai.profiles.channel_attached"
	AIProfilesChannelDetached  = "ai.profiles.channel_detached"
	AICredentialsCreated       = "ai.credentials.created"
	AICredentialsUpdated       = "ai.credentials.updated"
	AICredentialsDeleted       = "ai.credentials.deleted"
	AIPipelineCompleted        = "ai.pipeline.completed"
	AIPipelineEscalated        = "ai.pipeline.escalated"
	AIPipelineGuardFailed      = "ai.pipeline.guard_failed"
	AIDraftCreated             = "ai.draft.created"
	AIConversationTakeover     = "ai.conversation.takeover"
	AIConversationReleased     = "ai.conversation.released"
)

// Reports domain events (REPORTS_DOMAIN_ADR).
const (
	ReportsDefinitionsCreated = "reports.definitions.created"
	ReportsDashboardsCreated  = "reports.dashboards.created"
	ReportsExportCompleted    = "reports.export.completed"
)

// Tickets domain events (TICKETS_DOMAIN_ADR catalog).
const (
	TicketsCreated                      = "tickets.created"
	TicketsUpdated                      = "tickets.updated"
	TicketsStatusChanged                = "tickets.status_changed"
	TicketsDepartmentsCreated           = "tickets.departments.created"
	TicketsDepartmentsUpdated           = "tickets.departments.updated"
	TicketsDepartmentsDeleted           = "tickets.departments.deleted"
	TicketsDepartmentsMemberAdded       = "tickets.departments.member_added"
	TicketsDepartmentsMemberRemoved     = "tickets.departments.member_removed"
	TicketsCategoriesCreated            = "tickets.categories.created"
	TicketsCategoriesUpdated            = "tickets.categories.updated"
	TicketsCategoriesDeleted            = "tickets.categories.deleted"
	TicketsRulesTriggered               = "tickets.rules.triggered"
	TicketsRulesActionApplied           = "tickets.rules.action_applied"
	TicketsRemindersCreated             = "tickets.reminders.created"
	TicketsRemindersSent                = "tickets.reminders.sent"
	TicketsRemindersEscalated           = "tickets.reminders.escalated"
	TicketsSLAWarning                   = "tickets.sla.warning"
	TicketsSLABreached                  = "tickets.sla.breached"
	TicketsAssignmentsAssigned          = "tickets.assignments.assigned"
	TicketsAssignmentsReassigned        = "tickets.assignments.reassigned"
	TicketsAssignmentsUnassigned        = "tickets.assignments.unassigned"
	TicketsMessagesCreated              = "tickets.messages.created"
	TicketsConversationsTouched         = "tickets.conversations.touched"
	TicketsAttachmentsCreated           = "tickets.attachments.created"
	TicketsAttachmentsRead              = "tickets.attachments.read"
	TicketsAttachmentsAIDenied          = "tickets.attachments.ai_read_denied"
	TicketsEvaluationsLinkGenerated     = "tickets.evaluations.link_generated"
	TicketsEvaluationsFeedbackSubmitted = "tickets.evaluations.feedback_submitted"
	TicketsEvaluationsFinalized         = "tickets.evaluations.finalized"
	TicketsAISuggestionCreated          = "tickets.ai.suggestion_created"
	TicketsAIAssistEscalated            = "tickets.ai.assist_escalated"
	TicketsAIAssistGuardFailed          = "tickets.ai.assist_guard_failed"
)

// Contracts domain events.
const (
	ContractsInstanceSigned = "contracts.instance_signed"
)

// Auth / tenant notification source events (existing Notification Center templates).
const (
	AuthWelcome            = "auth.welcome"
	AuthEmailVerification  = "auth.email_verification"
	AuthPasswordReset      = "auth.password_reset"
	AuthProfileUpdated     = "auth.profile_updated"
	TenantMemberAdded      = "tenant.member_added"
	NotificationsTest      = "notifications.test"
	NotificationsQueued    = "notifications.queued"
	NotificationsSent      = "notifications.sent"
	NotificationsFailed    = "notifications.failed"
	NotificationsRead      = "notifications.read"
	NotificationsCancelled = "notifications.cancelled"
)

// ValidateEventName checks the naming standard without requiring catalog membership.
func ValidateEventName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("events: empty name")
	}
	if strings.Contains(name, " ") || strings.Contains(name, "/") {
		return fmt.Errorf("events: name %q must not contain spaces or slashes", name)
	}
	parts := strings.Split(name, ".")
	if len(parts) < 2 {
		return fmt.Errorf("events: name %q must match %s", name, NamingStandard)
	}
	for _, p := range parts {
		if p == "" {
			return fmt.Errorf("events: name %q has empty segment", name)
		}
		for _, r := range p {
			if unicode.IsUpper(r) {
				return fmt.Errorf("events: name %q must be lowercase", name)
			}
			if !unicode.IsLower(r) && !unicode.IsDigit(r) && r != '_' {
				return fmt.Errorf("events: name %q has invalid character in segment %q", name, p)
			}
		}
	}
	return nil
}

// AllKnownEvents returns every catalog constant (deduplicated, sorted).
func AllKnownEvents() []string {
	set := map[string]struct{}{}
	for _, n := range catalogConstants() {
		if n == "" {
			continue
		}
		set[n] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// IsKnownEvent reports whether name is in the platform catalog.
func IsKnownEvent(name string) bool {
	_, ok := knownEventSet()[name]
	return ok
}

func knownEventSet() map[string]struct{} {
	set := map[string]struct{}{}
	for _, n := range catalogConstants() {
		set[n] = struct{}{}
	}
	return set
}

func catalogConstants() []string {
	return []string{
		CariChargePosted,
		CariPaymentPosted,
		CariEntryVoided,
		JobsCreated,
		JobsReady,
		JobsDelivered,
		JobsClosed,
		JobsCancelled,
		JobsVoided,
		SalesCreated,
		SalesVoided,
		SuppliersCreated,
		SuppliersUpdated,
		SuppliersDeleted,
		PurchasesCreated,
		PurchasesVoided,
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
		ConversationsCreated,
		ConversationsUpdated,
		ConversationsAssigned,
		ConversationsResolved,
		ConversationsReopened,
		MessagesReceived,
		MessagesSent,
		MessagesStatusChanged,
		ConversationsTyping,
		ChannelAccountsConnected,
		ChannelAccountsDisconnected,
		ChannelAccountsError,
		CommerceAppsUpdated,
		CommerceConnectionsCreated,
		CommerceConnectionsUpdated,
		CommerceConnectionsDisconnected,
		CommerceConnectionsWorkspaceAdded,
		CommerceConnectionsWorkspaceRemoved,
		CommerceOrdersUpserted,
		CommerceOrdersLinkedToCustomer,
		CommerceRefundsUpdated,
		CommerceCategoriesUpserted,
		CommerceCategoriesDeleted,
		CommerceProductsUpserted,
		CommerceProductsDeleted,
		CommerceCatalogSyncCompleted,
		CommerceCatalogSyncFailed,
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
		ReportsDefinitionsCreated,
		ReportsDashboardsCreated,
		ReportsExportCompleted,
		TicketsCreated,
		TicketsUpdated,
		TicketsStatusChanged,
		TicketsDepartmentsCreated,
		TicketsDepartmentsUpdated,
		TicketsDepartmentsDeleted,
		TicketsDepartmentsMemberAdded,
		TicketsDepartmentsMemberRemoved,
		TicketsCategoriesCreated,
		TicketsCategoriesUpdated,
		TicketsCategoriesDeleted,
		TicketsRulesTriggered,
		TicketsRulesActionApplied,
		TicketsRemindersCreated,
		TicketsRemindersSent,
		TicketsRemindersEscalated,
		TicketsSLAWarning,
		TicketsSLABreached,
		TicketsAssignmentsAssigned,
		TicketsAssignmentsReassigned,
		TicketsAssignmentsUnassigned,
		TicketsMessagesCreated,
		TicketsConversationsTouched,
		TicketsAttachmentsCreated,
		TicketsAttachmentsRead,
		TicketsAttachmentsAIDenied,
		TicketsEvaluationsLinkGenerated,
		TicketsEvaluationsFeedbackSubmitted,
		TicketsEvaluationsFinalized,
		TicketsAISuggestionCreated,
		TicketsAIAssistEscalated,
		TicketsAIAssistGuardFailed,
		ContractsInstanceSigned,
		AuthWelcome,
		AuthEmailVerification,
		AuthPasswordReset,
		AuthProfileUpdated,
		TenantMemberAdded,
		NotificationsTest,
		NotificationsQueued,
		NotificationsSent,
		NotificationsFailed,
		NotificationsRead,
		NotificationsCancelled,
	}
}

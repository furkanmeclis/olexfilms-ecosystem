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
	// CariOpeningBalancePosted: the one-off opening balance of a cari
	// (TEC-177); its reversal publishes CariEntryVoided.
	CariOpeningBalancePosted = "cari.opening_balance_posted"
)

// Finance (income/expense ledger, TEC-99) domain events. A ledger row that
// settles or charges a cari publishes the cari.* event instead.
const (
	FinanceEntryPosted = "finance.entry_posted"
	FinanceEntryVoided = "finance.entry_voided"
)

// Accounting dispute events (TEC-174, K24): written to the outbox in the
// transaction that opens, resolves or rejects a dispute; notifications
// reach the parent (opened) and the disputing organization (resolved,
// rejected).
const (
	AccountingDisputeOpened   = "accounting.dispute_opened"
	AccountingDisputeResolved = "accounting.dispute_resolved"
	AccountingDisputeRejected = "accounting.dispute_rejected"
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

// Stock ledger domain events (TEC-154): one per movement type, written to
// the outbox by ledger.Post in the movement's transaction.
const (
	StockEntry                 = "stock.entry"
	StockPlacement             = "stock.placement"
	StockTransferOut           = "stock.transfer_out"
	StockTransferIn            = "stock.transfer_in"
	StockTransferCancelRestore = "stock.transfer_cancel_restore"
	StockOrderOut              = "stock.order_out"
	StockReceived              = "stock.received"
	StockOrderCancelRestore    = "stock.order_cancel_restore"
	StockConsumption           = "stock.consumption"
	StockPartialConsumption    = "stock.partial_consumption"
	StockReturn                = "stock.return"
	StockSale                  = "stock.sale"
	StockReclassification      = "stock.reclassification"
	StockCountAdjustment       = "stock.count_adjustment"
	StockVoid                  = "stock.void"
	StockExternalOutbound      = "stock.external_outbound"
	StockSplit                 = "stock.split"
)

// Order domain events (TEC-165): one per status transition, written to the
// outbox in the transition's transaction. Distinct from the commerce.*
// events of external commerce apps.
const (
	OrdersCreated         = "orders.created"
	OrdersUpdated         = "orders.updated"
	OrdersSubmitted       = "orders.submitted"
	OrdersApproved        = "orders.approved"
	OrdersPreparing       = "orders.preparing"
	OrdersReady           = "orders.ready"
	OrdersProcessing      = "orders.processing"
	OrdersShipped         = "orders.shipped"
	OrdersDelivered       = "orders.delivered"
	OrdersReceived        = "orders.received"
	OrdersCancelRequested = "orders.cancel_requested"
	OrdersCancelled       = "orders.cancelled"
)

// Announcement domain events (TEC-330): publishing writes one outbox event
// when notify=true; the notification center fans it out to the resolved
// audience over in-app, e-mail and push (not WhatsApp).
const (
	AnnouncementPublished = "announcement.published"
)

// Sibling transfer events (K13, TEC-165; TEC-197): one per status
// transition of a stock transfer request, written to the outbox in the
// transition's transaction. transfers.completed is the 000049 name and is
// not written since TEC-197 (shipped/received replace it).
const (
	TransfersRequested = "transfers.requested"
	TransfersApproved  = "transfers.approved"
	TransfersRejected  = "transfers.rejected"
	TransfersShipped   = "transfers.shipped"
	TransfersReceived  = "transfers.received"
	TransfersCompleted = "transfers.completed"
	TransfersCancelled = "transfers.cancelled"
)

// Service domain events (TEC-97 / TEC-178): one per status transition,
// written to the outbox in the transition's transaction. service.completed
// is written in the completion transaction together with the stock
// consumption; TEC-98 opens the warranty from it (decision 7).
const (
	ServiceCreated      = "service.created"
	ServiceUpdated      = "service.updated"
	ServicePending      = "service.pending"
	ServiceProcessing   = "service.processing"
	ServiceReady        = "service.ready"
	ServiceCompleted    = "service.completed"
	ServiceCancelled    = "service.cancelled"
	ServiceImageAdded   = "service.image_added"
	ServiceImageRemoved = "service.image_removed"
	ServiceNoteAdded    = "service.note_added"
)

// Certificate add-on events (TEC-480/F5-03b).
const (
	CertificateUploaded       = "certificate.uploaded"
	CertificateVerified       = "certificate.verified"
	CertificateRejected       = "certificate.rejected"
	CertificateExpired        = "certificate.expired"
	CertificateRevoked        = "certificate.revoked"
	CertificateServiceWarning = "certificate.service_warning"
)

// MeasurementDiffCheckRequired (TEC-297) is written when a service's
// before/after micron difference deviates from the product expectation and
// the dealer owner should review the measurement table.
const MeasurementDiffCheckRequired = "measurement.diff_check_required"

// Service subscription events (TEC-307): assignment, early cancellation
// request and final center decision. Notifications consume all three; the
// cancellation approval also lets downstream accounting/search consumers see
// a single durable service_subscription.cancelled event.
const (
	ServiceSubscriptionAssigned        = "service_subscription.assigned"
	ServiceSubscriptionCancelRequested = "service_subscription.cancel_requested"
	ServiceSubscriptionCancelled       = "service_subscription.cancelled"
	ServiceSubscriptionCancelRejected  = "service_subscription.cancel_rejected"
)

// Appointment domain events (TEC-323): written when a booking is created,
// rescheduled or cancelled.
const (
	AppointmentCreated     = "appointment.created"
	AppointmentRescheduled = "appointment.rescheduled"
	AppointmentCancelled   = "appointment.cancelled"
	AppointmentReminder    = "appointment.reminder"
	AppointmentNoShow      = "appointment.no_show"
)

// ServiceReviewRequested is written by the delayed service:review_request
// task (TEC-192) in the transaction that stamps review_request_sent_at; the
// notification module sends the WhatsApp review request from it.
const ServiceReviewRequested = "service.review_requested"

// ServiceReviewed is written when the customer submits the platform review
// form. Processing and reporting consume this event asynchronously.
const ServiceReviewed = "service.reviewed"

// ServiceReviewLowScore is written by review processing when a review falls
// at or below the low-score threshold; notification center sends it to the
// dealer owner(s).
const ServiceReviewLowScore = "service.review.low_score"

// Warranty domain events (TEC-98 / TEC-185). warranty.created is written
// by the service.completed consumer, expiring_soon (payload days: 30 or 7)
// and expired by the daily cron, holder_changed by a completed vehicle
// transfer, voided by the center.
const (
	WarrantyCreated            = "warranty.created"
	WarrantyExpiringSoon       = "warranty.expiring_soon"
	WarrantyExpired            = "warranty.expired"
	WarrantyVoided             = "warranty.voided"
	WarrantyHolderChanged      = "warranty.holder_changed"
	WarrantyClaimStatusChanged = "warranty_claim.status_changed"
)

// Vehicle ownership transfer events (TEC-98 decision 6): two codes, one
// for the current owner and one for the new owner.
const (
	VehicleTransferStarted   = "vehicle.transfer_started"
	VehicleTransferCompleted = "vehicle.transfer_completed"
	VehicleTransferCancelled = "vehicle.transfer_cancelled"
	VehicleTransferExpired   = "vehicle.transfer_expired"
)

// Customer merge (TEC-193): the center folded a duplicate user (entity) into
// another; payload carries both uuids and the moved record counts.
const (
	CustomerMerged = "customer.merged"
)

// Center task events (TEC-214): written to the outbox in the transaction
// that changes the task; the notification catalog (TEC-221) reaches the
// assignee.
const (
	TasksCreated       = "tasks.created"
	TasksUpdated       = "tasks.updated"
	TasksAssigned      = "tasks.assigned"
	TasksStatusChanged = "tasks.status_changed"
	TasksCommentAdded  = "tasks.comment_added"
)

// CustomerCreated (TEC-164) is written when an organization creates a new
// customer user; the notification module sends the WhatsApp welcome with
// the portal link.
const CustomerCreated = "customer.created"

// Contracts domain events.
const (
	ContractsInstanceSigned = "contracts.instance_signed"
	ContractExecuted        = "contract.executed"
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

// Vehicle record events (TEC-209): written in the transaction that creates,
// edits or soft-deletes a vehicle; the search sync refreshes the vehicles
// index document from them.
const (
	VehicleCreated = "vehicle.created"
	VehicleUpdated = "vehicle.updated"
	VehicleDeleted = "vehicle.deleted"
)

// Organization record events (TEC-210): written in the transaction that
// registers an organization or edits its name, dealer code, address or
// parent; the search sync refreshes the organizations index from them.
const (
	OrganizationCreated = "organization.created"
	OrganizationUpdated = "organization.updated"
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
		CariOpeningBalancePosted,
		FinanceEntryPosted,
		FinanceEntryVoided,
		AccountingDisputeOpened,
		AccountingDisputeResolved,
		AccountingDisputeRejected,
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
		StockEntry,
		StockPlacement,
		StockTransferOut,
		StockTransferIn,
		StockTransferCancelRestore,
		StockOrderOut,
		StockReceived,
		StockOrderCancelRestore,
		StockConsumption,
		StockPartialConsumption,
		StockReturn,
		StockSale,
		StockReclassification,
		StockCountAdjustment,
		StockVoid,
		StockExternalOutbound,
		StockSplit,
		OrdersCreated,
		OrdersUpdated,
		OrdersSubmitted,
		OrdersApproved,
		OrdersPreparing,
		OrdersReady,
		OrdersProcessing,
		OrdersShipped,
		OrdersDelivered,
		OrdersReceived,
		OrdersCancelRequested,
		OrdersCancelled,
		TransfersRequested,
		TransfersApproved,
		TransfersRejected,
		TransfersShipped,
		TransfersReceived,
		TransfersCompleted,
		TransfersCancelled,
		ServiceCreated,
		ServiceUpdated,
		ServicePending,
		ServiceProcessing,
		ServiceReady,
		ServiceCompleted,
		ServiceCancelled,
		ServiceImageAdded,
		ServiceImageRemoved,
		ServiceNoteAdded,
		CertificateUploaded,
		CertificateVerified,
		CertificateRejected,
		CertificateExpired,
		CertificateRevoked,
		CertificateServiceWarning,
		AppointmentCreated,
		AppointmentRescheduled,
		AppointmentCancelled,
		AppointmentReminder,
		AppointmentNoShow,
		ServiceReviewRequested,
		WarrantyCreated,
		WarrantyExpiringSoon,
		WarrantyExpired,
		WarrantyVoided,
		WarrantyHolderChanged,
		VehicleTransferStarted,
		VehicleTransferCompleted,
		VehicleTransferCancelled,
		VehicleTransferExpired,
		CustomerMerged,
		CustomerCreated,
		ContractsInstanceSigned,
		ContractExecuted,
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
		TasksCreated,
		TasksUpdated,
		TasksAssigned,
		TasksStatusChanged,
		TasksCommentAdded,
		VehicleCreated,
		VehicleUpdated,
		VehicleDeleted,
		TasksDueSoon,
		TasksOverdue,
		OrganizationCreated,
		OrganizationUpdated,
		QuoteSent,
		LeadsApplicationReceived,
		MeasurementMatchSuggested,
		MeasurementDiffCheckRequired,
		CampaignsSubmitted,
		CampaignsApproved,
		CampaignsRejected,
		CampaignsChangesRequested,
		AIQuotaThreshold,
		CampaignsStarted,
		CampaignsFinished,
		FleetCreated,
		FleetLinkRequested,
		FleetLinked,
		FleetLinkRejected,
		FleetVehicleAdded,
		FleetUserInvited,
		FleetServicePlanCreated,
		ShowcaseReviewRequested,
		ShowcasePublished,
		ShowcaseRejected,
	}
}

// Center task due date reminders (TEC-221): written by tasks:due_scan once
// per task and threshold (tasks.due_soon_notified_at / overdue_notified_at);
// the notification catalog reaches the assignee (or the creator).
const (
	TasksDueSoon = "tasks.due_soon"
	TasksOverdue = "tasks.overdue"
)

// LeadsApplicationReceived (TEC-317) is written when the public dealer
// application form opens a lead; tenant is the receiving organization
// (territory distributor or brand center) and notify_user_ids its members
// holding leads.read.
const LeadsApplicationReceived = "leads.application_received"

// QuoteSent (TEC-315) is written when a quote is sent or reminded over
// WhatsApp. Payload carries the recipient phone and public quote URL.
const QuoteSent = "quote.sent"

// MeasurementMatchSuggested (TEC-296): the before/after matching of a
// service linked a measurement automatically (waiting for the dealer's
// confirmation) or found candidates; written in the matching transaction.
const MeasurementMatchSuggested = "measurement.match_suggested"

// Campaign approval chain (TEC-406, F4-04c): written in the transaction that
// moves the campaign; notify_user_ids are the approver organization's
// members holding campaigns.approve (submitted) or the campaign creator
// (approved / rejected / changes_requested).
const (
	CampaignsSubmitted        = "campaigns.submitted"
	CampaignsApproved         = "campaigns.approved"
	CampaignsRejected         = "campaigns.rejected"
	CampaignsChangesRequested = "campaigns.changes_requested"
)

// Campaign sending (TEC-407, F4-04d): started is written when the scheduler
// moves a due campaign to sending and takes the recipient snapshot;
// finished when the last recipient ends (sent / partially_failed). Payload:
// campaign_uuid, brand_id, organization_id, status, recipients_* counters.
const (
	CampaignsStarted  = "campaigns.started"
	CampaignsFinished = "campaigns.finished"
)

// AIQuotaThreshold (TEC-388) is written in the usage transaction when a
// model call moves an organization's monthly AI token pool past 80 % or
// 100 % of its quota: once per threshold, pool and month, since the
// monthly projection only grows. Payload: organization_id, brand_id, pool,
// period, threshold (80 | 100), used_tokens, quota_tokens. The
// notification template reaches the organization owners (F4-01g).
const AIQuotaThreshold = "ai.quota.threshold"

// Fleet management (TEC-473, F5-02b): written in the transaction that opens
// a fleet, requests or decides a dealer link, adds a vehicle to a fleet or
// invites a fleet user. tenant is the acting organization (the fleet for a
// portal decision); payload: fleet_uuid, fleet_id, brand_id, fleet_name
// and, per event, link_uuid / dealer_org_id / dealer_name, vehicle_uuid,
// notify_user_ids (link requests reach the fleet users, decisions the
// requesting user). The search sync refreshes the fleet document.
const (
	FleetCreated            = "fleet.created"
	FleetLinkRequested      = "fleet.link_requested"
	FleetLinked             = "fleet.linked"
	FleetLinkRejected       = "fleet.link_rejected"
	FleetVehicleAdded       = "fleet.vehicle_added"
	FleetUserInvited        = "fleet.user_invited"
	FleetServicePlanCreated = "fleet.service_plan_created"
)

// Dealer showcase (TEC-467, F5-01b): written in the transaction that moves
// the showcase. review_requested when an owner submits with
// showcase.approval_required on (notify_user_ids: the brand center's
// members holding platform.showcase.review); published when the snapshot is
// written (direct publish or center approval; notify_user_ids: the owners
// of the organization after an approval, empty on a direct publish);
// rejected with the reviewer's note (notify_user_ids: the owners). Payload:
// organization_uuid, organization_id, organization_name, brand_id, status,
// reason. The organizations index refreshes has_showcase on published.
const (
	ShowcaseReviewRequested = "showcase.review_requested"
	ShowcasePublished       = "showcase.published"
	ShowcaseRejected        = "showcase.rejected"
)

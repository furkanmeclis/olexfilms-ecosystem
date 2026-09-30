/**
 * Domain event type strings broadcast via Centrifugo.
 * Source of truth: api/internal/platform/events (RealtimeEvents).
 * Features must use these constants — no magic strings.
 */
export const RealtimeEvents = {
  // Users / auth
  UserCreated: "users.created",
  UserUpdated: "users.updated",
  UserDeleted: "users.deleted",
  UserLoggedIn: "auth.login",

  // Dealers
  DealerCreated: "dealer.created",
  DealerUpdated: "dealer.updated",
  DealerDisabled: "dealer.disabled",
  DealerDeleted: "dealer.deleted",

  // Roles
  RoleAssigned: "roles.role_changed",
  RoleCreated: "roles.created",
  RoleUpdated: "roles.updated",
  RoleDeleted: "roles.deleted",
  PermissionChanged: "permissions.permission_updated",

  // Customers / CRM
  CustomerCreated: "customers.created",
  CustomerUpdated: "customers.updated",
  CustomerDeleted: "customers.deleted",

  // Inbox
  ConversationsCreated: "conversations.created",
  ConversationsUpdated: "conversations.updated",
  ConversationsAssigned: "conversations.assigned",
  ConversationsResolved: "conversations.resolved",
  ConversationsReopened: "conversations.reopened",
  MessagesReceived: "messages.received",
  MessagesSent: "messages.sent",
  MessagesStatusChanged: "messages.status_changed",
  ConversationsTyping: "conversations.typing",

  // AI pipeline / drafts (inbox ops)
  AIPipelineCompleted: "ai.pipeline.completed",
  AIPipelineEscalated: "ai.pipeline.escalated",
  AIPipelineGuardFailed: "ai.pipeline.guard_failed",
  AIDraftCreated: "ai.draft.created",
  AIConversationTakeover: "ai.conversation.takeover",
  AIConversationReleased: "ai.conversation.released",

  // Reservations
  ReservationCreated: "reservations.created",
  ReservationConfirmed: "reservations.confirmed",
  ReservationCancelled: "reservations.cancelled",
  ReservationCompleted: "reservations.completed",

  // Notifications (lifecycle domain events)
  NotificationQueued: "notifications.queued",
  NotificationSent: "notifications.sent",
  NotificationDelivered: "notifications.delivered",
  NotificationRead: "notifications.read",
  NotificationFailed: "notifications.failed",
  NotificationCancelled: "notifications.cancelled",
  // In-app / Centrifugo delivery events (NOTIFICATION_CENTER.md)
  NotificationItemCreated: "notification.created",
  NotificationItemUpdated: "notification.updated",
  NotificationItemRead: "notification.read",

  // Jobs
  JobQueued: "jobs.queued",
  JobStarted: "jobs.started",
  JobCompleted: "jobs.completed",
  JobFailed: "jobs.failed",
  JobCancelled: "jobs.cancelled",

  // Imports
  ImportRequested: "imports.requested",
  ImportStarted: "imports.started",
  ImportCompleted: "imports.completed",
  ImportFailed: "imports.failed",

  // Exports
  ExportRequested: "exports.requested",
  ExportStarted: "exports.started",
  ExportCompleted: "exports.completed",
  ExportFailed: "exports.failed",
  ExportCancelled: "exports.cancelled",
  ReportExportCompleted: "reports.export_completed",
  ReportExportFailed: "reports.export_failed",
} as const;

export type RealtimeEventType =
  (typeof RealtimeEvents)[keyof typeof RealtimeEvents];

/** Events that the default Notification Stream surfaces as toasts. */
export const ToastableRealtimeEvents: ReadonlySet<string> = new Set([
  RealtimeEvents.NotificationFailed,
  RealtimeEvents.JobCompleted,
  RealtimeEvents.JobFailed,
  RealtimeEvents.ImportCompleted,
  RealtimeEvents.ImportFailed,
  RealtimeEvents.ExportCompleted,
  RealtimeEvents.ExportFailed,
  RealtimeEvents.ExportCancelled,
  RealtimeEvents.ReportExportCompleted,
  RealtimeEvents.ReportExportFailed,
]);

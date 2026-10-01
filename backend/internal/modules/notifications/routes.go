package notifications

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// RegisterRoutes mounts notification routes.
func RegisterRoutes(
	mux *http.ServeMux,
	h *handler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
) {
	authn := middleware.Authenticate(tokens, loader)
	requirePerm := func(slug string) func(http.Handler) http.Handler {
		return middleware.RequirePermission(slug)
	}

	mux.Handle("GET /v1/notifications/vapid-public-key", middleware.Chain(
		http.HandlerFunc(h.VAPIDPublicKey), authn,
	))
	mux.Handle("POST /v1/notifications/push-subscriptions", middleware.Chain(
		http.HandlerFunc(h.UpsertPushSubscription), authn,
	))
	mux.Handle("DELETE /v1/notifications/push-subscriptions", middleware.Chain(
		http.HandlerFunc(h.DeletePushSubscription), authn,
	))

	mux.Handle("POST /v1/notifications/push-devices", middleware.Chain(
		http.HandlerFunc(h.RegisterPushDevice), authn,
	))
	mux.Handle("DELETE /v1/notifications/push-devices", middleware.Chain(
		http.HandlerFunc(h.RevokePushDevice), authn,
	))
	mux.Handle("GET /v1/notification-events", middleware.Chain(
		http.HandlerFunc(h.Events), authn,
	))

	mux.Handle("GET /v1/notifications/meta", middleware.Chain(
		http.HandlerFunc(h.Meta), authn, requirePerm(rbac.PermNotificationsRead),
	))
	mux.Handle("GET /v1/notifications/unread-count", middleware.Chain(
		http.HandlerFunc(h.UnreadCount), authn, requirePerm(rbac.PermNotificationsRead),
	))
	mux.Handle("POST /v1/notifications/read-all", middleware.Chain(
		http.HandlerFunc(h.MarkAllRead), authn, requirePerm(rbac.PermNotificationsRead),
	))
	mux.Handle("POST /v1/notifications/test", middleware.Chain(
		http.HandlerFunc(h.Test), authn,
	))
	mux.Handle("POST /v1/notifications/send", middleware.Chain(
		http.HandlerFunc(h.Send), authn, requirePerm(rbac.PermNotificationsManage),
	))
	mux.Handle("GET /v1/notifications", middleware.Chain(
		http.HandlerFunc(h.List), authn, requirePerm(rbac.PermNotificationsRead),
	))
	mux.Handle("GET /v1/notifications/{uuid}", middleware.Chain(
		http.HandlerFunc(h.Get), authn, requirePerm(rbac.PermNotificationsRead),
	))
	mux.Handle("GET /v1/notifications/{uuid}/action", middleware.Chain(
		http.HandlerFunc(h.FollowAction), authn, requirePerm(rbac.PermNotificationsRead),
	))
	mux.Handle("POST /v1/notifications/{uuid}/read", middleware.Chain(
		http.HandlerFunc(h.MarkRead), authn, requirePerm(rbac.PermNotificationsRead),
	))

	mux.Handle("GET /v1/notification-preferences", middleware.Chain(
		http.HandlerFunc(h.GetPreferences), authn,
	))
	mux.Handle("PUT /v1/notification-preferences", middleware.Chain(
		http.HandlerFunc(h.PutPreferences), authn,
	))

	mux.Handle("GET /v1/platform/notifications/meta", middleware.Chain(
		http.HandlerFunc(h.PlatformMeta), authn, requirePerm(rbac.PermPlatformNotificationsRead),
	))
	mux.Handle("GET /v1/platform/notifications", middleware.Chain(
		http.HandlerFunc(h.ListPlatform), authn, requirePerm(rbac.PermPlatformNotificationsRead),
	))

	// Notification center admin (TEC-87).
	manage := requirePerm(rbac.PermNotificationTemplatesManage)
	mux.Handle("GET /v1/platform/notification-templates", middleware.Chain(
		http.HandlerFunc(h.ListTemplates), authn, manage,
	))
	mux.Handle("PUT /v1/platform/notification-templates", middleware.Chain(
		http.HandlerFunc(h.UpsertTemplate), authn, manage,
	))
	mux.Handle("POST /v1/platform/notification-templates/preview", middleware.Chain(
		http.HandlerFunc(h.PreviewTemplate), authn, manage,
	))
	mux.Handle("GET /v1/platform/notification-channels", middleware.Chain(
		http.HandlerFunc(h.ListChannels), authn, manage,
	))
	mux.Handle("PUT /v1/platform/notification-channels/{channel}", middleware.Chain(
		http.HandlerFunc(h.SetChannel), authn, manage,
	))
	mux.Handle("GET /v1/platform/notification-deliveries", middleware.Chain(
		http.HandlerFunc(h.ListDeliveries), authn, requirePerm(rbac.PermNotificationDeliveriesRead),
	))
}

package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
)

// TEC-87: the "Talep et" request goes through the catalog event
// features.module_requested (template + delivery row), and the admin API
// is guarded by notifications.templates.manage / notification_deliveries.read.
func TestIntegrationNotificationCenterHTTP(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	net := it.featureNet(1)
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "UPDATE notification_channel_settings SET enabled = TRUE WHERE channel = 'expo_push'")
	})

	code, env := it.do("POST", "/v1/features/"+features.ModuleStockForecast+"/request", hostOlex, net.dealerTok, map[string]any{"note": "please"})
	if code != http.StatusAccepted {
		t.Fatalf("request = %d %s", code, errCode(env))
	}
	var status, title, role string
	err := it.pool.QueryRow(ctx, `
		SELECT d.status, n.title, d.role
		FROM notification_deliveries d
		JOIN notifications n ON n.delivery_id = d.id
		JOIN organization_members om ON om.user_id = d.user_id AND om.organization_id = $1
		WHERE d.event_code = 'features.module_requested' AND d.channel = 'inapp'
		ORDER BY d.id DESC LIMIT 1`, net.dist.ID).Scan(&status, &title, &role)
	if err != nil {
		t.Fatalf("module request delivery: %v", err)
	}
	if status != "delivered" || title != "Modül talebi" || role != "distributor" {
		t.Fatalf("delivery = %s %q %s", status, title, role)
	}

	// Guarded by notifications.templates.manage.
	if code, ec := it.status("GET", "/v1/platform/notification-templates", net.dealerTok); code != http.StatusForbidden {
		t.Fatalf("dealer templates = %d %s", code, ec)
	}
	code, env = it.do("GET", "/v1/platform/notification-channels", hostOlex, net.admin, nil)
	if code != http.StatusOK {
		t.Fatalf("channels = %d %s", code, errCode(env))
	}
	var chans struct {
		Items []struct {
			Channel string `json:"channel"`
			Enabled bool   `json:"enabled"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &chans)
	if len(chans.Items) != 6 {
		t.Fatalf("channels = %s", env.Data)
	}
	// expo_push: the SMS switch is exercised by the usecase test, and both
	// packages run in parallel against one database.
	code, env = it.do("PUT", "/v1/platform/notification-channels/expo_push", hostOlex, net.admin, map[string]any{"enabled": false})
	if code != http.StatusOK || !json.Valid(env.Data) {
		t.Fatalf("expo_push off = %d %s", code, errCode(env))
	}
	if code, _ := it.do("PUT", "/v1/platform/notification-channels/fax", hostOlex, net.admin, map[string]any{"enabled": true}); code != http.StatusBadRequest {
		t.Fatalf("unknown channel = %d", code)
	}
	code, env = it.do("PUT", "/v1/platform/notification-templates", hostOlex, net.admin, map[string]any{
		"code": "features.module_requested", "role": "dealer", "channel": "email", "language": "ar",
		"subject": "x", "body": "{{module_key}} {{oops}}",
	})
	if code != http.StatusBadRequest {
		t.Fatalf("unknown placeholder = %d %s", code, errCode(env))
	}
	code, env = it.do("POST", "/v1/platform/notification-templates/preview", hostOlex, net.admin, map[string]any{
		"code": "features.module_requested", "channel": "email", "language": "ar",
		"subject": "{{module_key}}", "body": "**{{organization_name}}**",
	})
	if code != http.StatusOK {
		t.Fatalf("preview = %d %s", code, errCode(env))
	}
	code, env = it.do("GET", "/v1/platform/notification-deliveries?event_code=features.module_requested", hostOlex, net.admin, nil)
	if code != http.StatusOK {
		t.Fatalf("deliveries = %d %s", code, errCode(env))
	}
	var page struct {
		Total int64 `json:"total"`
	}
	_ = json.Unmarshal(env.Data, &page)
	if page.Total < 1 {
		t.Fatalf("deliveries page = %s", env.Data)
	}
	if code, ec := it.status("GET", "/v1/notification-events", net.dealerTok); code != http.StatusOK {
		t.Fatalf("events = %d %s", code, ec)
	}
}

package templates_test

import (
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/templates"
)

func TestRender(t *testing.T) {
	t.Parallel()
	got := templates.Render("Hi {{name}}, code {{code}}", map[string]string{"name": "Ada", "code": "123456"})
	if got != "Hi Ada, code 123456" {
		t.Fatalf("got %q", got)
	}
}

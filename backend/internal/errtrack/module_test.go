package errtrack

import "testing"

func TestModuleListIsFixed(t *testing.T) {
	want := []string{
		"auth", "org", "catalog", "inventory", "order", "service", "warranty",
		"accounting", "notification", "whatsapp", "ai", "mcp", "migrator", "docs",
	}
	got := Modules()
	if len(got) != len(want) {
		t.Fatalf("module count %d, want %d", len(got), len(want))
	}
	for i, m := range got {
		if string(m) != want[i] || !m.Valid() {
			t.Errorf("module %d = %q, want %q", i, m, want[i])
		}
		if ParseModule(" "+want[i]+" ") != m {
			t.Errorf("ParseModule(%q) mismatch", want[i])
		}
	}
}

func TestUnknownModule(t *testing.T) {
	for _, s := range []string{"", "billing", "Unknown", "auth2", "settings"} {
		if got := ParseModule(s); got != ModuleUnknown {
			t.Errorf("ParseModule(%q) = %q, want unknown", s, got)
		}
	}
	if ModuleUnknown.Valid() {
		t.Error("unknown must not be in the fixed list")
	}
	if Module("crm").Normalize() != ModuleUnknown {
		t.Error("Normalize must map unknown values")
	}
}

func TestModuleFromPath(t *testing.T) {
	cases := map[string]Module{
		"/api/v1/auth/login":                 ModuleAuth,
		"/api/v1/platform/users/1":           ModuleAuth,
		"/api/v1/platform/organizations/x":   ModuleOrg,
		"/api/v1/public/brand":               ModuleOrg,
		"/api/v1/notifications/unread-count": ModuleNotification,
		"/api/v1/tenant/exports":             ModuleDocs,
		"/api/v1/tenant/orders/5":            ModuleOrder,
		"/mcp/dealer":                        ModuleMCP,
		"/hooks/wuzapi":                      ModuleWhatsApp,
		"/api/v1/platform/storage/files":     ModuleUnknown,
		"/healthz":                           ModuleUnknown,
		"/":                                  ModuleUnknown,
	}
	for p, want := range cases {
		if got := ModuleFromPath(p); got != want {
			t.Errorf("ModuleFromPath(%q) = %q, want %q", p, got, want)
		}
	}
}

func TestModuleForTask(t *testing.T) {
	cases := map[string]Module{
		"app:notification:deliver": ModuleNotification,
		"app:export:process":       ModuleDocs,
		"catalog:sync":             ModuleCatalog,
		"app:logs:purge_sweep":     ModuleUnknown,
		"app:ping":                 ModuleUnknown,
		"":                         ModuleUnknown,
	}
	for tt, want := range cases {
		if got := ModuleForTask(tt); got != want {
			t.Errorf("ModuleForTask(%q) = %q, want %q", tt, got, want)
		}
	}
}

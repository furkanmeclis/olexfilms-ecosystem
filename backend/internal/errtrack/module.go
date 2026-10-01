package errtrack

import (
	"context"
	"strings"
)

// Module is the error category attached to every event as the "module" tag
// (K17: categories are fixed in code, the receiver only knows free tags).
type Module string

// The fixed module list. Anything else is reported as ModuleUnknown.
const (
	ModuleAuth         Module = "auth"
	ModuleOrg          Module = "org"
	ModuleCatalog      Module = "catalog"
	ModuleInventory    Module = "inventory"
	ModuleOrder        Module = "order"
	ModuleService      Module = "service"
	ModuleWarranty     Module = "warranty"
	ModuleAccounting   Module = "accounting"
	ModuleNotification Module = "notification"
	ModuleWhatsApp     Module = "whatsapp"
	ModuleAI           Module = "ai"
	ModuleMCP          Module = "mcp"
	ModuleMigrator     Module = "migrator"
	ModuleDocs         Module = "docs"
	ModuleUnknown      Module = "unknown"
)

// Modules returns the fixed module list (without ModuleUnknown).
func Modules() []Module {
	return []Module{
		ModuleAuth, ModuleOrg, ModuleCatalog, ModuleInventory, ModuleOrder,
		ModuleService, ModuleWarranty, ModuleAccounting, ModuleNotification,
		ModuleWhatsApp, ModuleAI, ModuleMCP, ModuleMigrator, ModuleDocs,
	}
}

var knownModules = func() map[Module]struct{} {
	m := make(map[Module]struct{}, 14)
	for _, mod := range Modules() {
		m[mod] = struct{}{}
	}
	return m
}()

// Valid reports whether m is one of the fixed modules.
func (m Module) Valid() bool {
	_, ok := knownModules[m]
	return ok
}

// Normalize returns m when it is a fixed module, ModuleUnknown otherwise.
func (m Module) Normalize() Module {
	if m.Valid() {
		return m
	}
	return ModuleUnknown
}

// ParseModule maps a free string to a fixed module (unknown → ModuleUnknown).
func ParseModule(s string) Module {
	return Module(strings.ToLower(strings.TrimSpace(s))).Normalize()
}

// segmentAliases maps URL segments / task type segments to modules. Only
// segments listed here (or equal to a module name) resolve; the rest is
// reported as unknown.
var segmentAliases = map[string]Module{
	"auth":                     ModuleAuth,
	"me":                       ModuleAuth,
	"users":                    ModuleAuth,
	"roles":                    ModuleAuth,
	"permissions":              ModuleAuth,
	"access":                   ModuleAuth,
	"sessions":                 ModuleAuth,
	"org":                      ModuleOrg,
	"orgs":                     ModuleOrg,
	"organizations":            ModuleOrg,
	"brand":                    ModuleOrg,
	"brands":                   ModuleOrg,
	"catalog":                  ModuleCatalog,
	"products":                 ModuleCatalog,
	"inventory":                ModuleInventory,
	"stock":                    ModuleInventory,
	"warehouses":               ModuleInventory,
	"order":                    ModuleOrder,
	"orders":                   ModuleOrder,
	"service":                  ModuleService,
	"services":                 ModuleService,
	"warranty":                 ModuleWarranty,
	"warranties":               ModuleWarranty,
	"accounting":               ModuleAccounting,
	"ledger":                   ModuleAccounting,
	"notification":             ModuleNotification,
	"notifications":            ModuleNotification,
	"notification-preferences": ModuleNotification,
	"whatsapp":                 ModuleWhatsApp,
	"wuzapi":                   ModuleWhatsApp,
	"ai":                       ModuleAI,
	"mcp":                      ModuleMCP,
	"migrator":                 ModuleMigrator,
	"docs":                     ModuleDocs,
	"pdf":                      ModuleDocs,
	"exports":                  ModuleDocs,
	"export":                   ModuleDocs,
}

// scopeSegments are path segments that only carry the audience (platform,
// tenant...) and are skipped when looking for the module segment.
var scopeSegments = map[string]struct{}{
	"api": {}, "v1": {}, "platform": {}, "tenant": {}, "public": {}, "internal": {},
}

func moduleFromSegment(seg string) Module {
	if m, ok := segmentAliases[strings.ToLower(seg)]; ok {
		return m
	}
	return ModuleUnknown
}

// ModuleFromPath derives the module from a request path such as
// /api/v1/platform/organizations/{id} → org, /mcp/dealer → mcp,
// /hooks/wuzapi → whatsapp. Unmapped paths are ModuleUnknown.
func ModuleFromPath(path string) Module {
	for _, seg := range strings.Split(path, "/") {
		if seg == "" {
			continue
		}
		if _, skip := scopeSegments[strings.ToLower(seg)]; skip {
			continue
		}
		if strings.EqualFold(seg, "hooks") {
			continue
		}
		return moduleFromSegment(seg)
	}
	return ModuleUnknown
}

// ModuleForTask derives the module from an Asynq task type such as
// "app:notification:deliver" (→ notification) or "catalog:sync" (→ catalog).
func ModuleForTask(taskType string) Module {
	for _, seg := range strings.Split(taskType, ":") {
		if seg == "" || seg == "app" {
			continue
		}
		return moduleFromSegment(seg)
	}
	return ModuleUnknown
}

type moduleCtxKey struct{}

// WithModule pins the module for errors captured with ctx (overrides the
// path / task type guess). Unknown values are normalized.
func WithModule(ctx context.Context, m Module) context.Context {
	return context.WithValue(ctx, moduleCtxKey{}, m.Normalize())
}

// ModuleFrom returns the module pinned with WithModule.
func ModuleFrom(ctx context.Context) (Module, bool) {
	m, ok := ctx.Value(moduleCtxKey{}).(Module)
	return m, ok
}

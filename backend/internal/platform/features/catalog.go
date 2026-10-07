// Package features is the module (feature flag) layer of TEC-86.
//
// Modules come in three levels (docs/design.md §4 "Modül paketleri", §5):
// core modules can never be switched off, standard modules are on by default
// and may be switched off, add-ons are off by default and are switched on by
// a higher level. The catalog in this file is the single source of truth for
// module keys: migration 000031 seeds the same rows, SyncCatalog reconciles a
// database at start-up and the frontend reads the list from GET /v1/features.
package features

// Level is the package level of a module.
type Level string

const (
	LevelCore     Level = "core"
	LevelStandard Level = "standard"
	LevelAddon    Level = "addon"
)

// ModuleDef is one catalog entry. DefaultEnabled and Paid are only the seed
// values: the platform admin may change them later (modules table).
type ModuleDef struct {
	Key            string
	Level          Level
	DefaultEnabled bool
	Paid           bool
}

// Module keys. Business modules that have no routes yet are listed so routes,
// menus and guards can reference them as they arrive.
const (
	// Core (§5.1).
	ModuleOrganizations  = "organizations"
	ModuleRegions        = "regions"
	ModuleCatalog        = "catalog"
	ModuleStock          = "stock"
	ModuleWarehouse      = "warehouse"
	ModuleOrders         = "orders"
	ModuleServices       = "services"
	ModuleCustomers      = "customers"
	ModuleNotifications  = "notifications"
	ModuleAccounting     = "accounting"
	ModuleSearch         = "search"
	ModuleImportExport   = "import_export"
	ModuleTasks          = "tasks"
	ModuleSystemSettings = "system_settings"

	// Standard (§5.2).
	ModuleIntakeContracts  = "intake_contracts"
	ModuleMeasurements     = "measurements"
	ModuleLeads            = "leads"
	ModuleAppointments     = "appointments"
	ModuleAnnouncements    = "announcements"
	ModuleWarrantyClaims   = "warranty_claims"
	ModuleDealerAccounting = "dealer_accounting"
	ModuleServiceCatalog   = "service_catalog"
	ModuleDealerTransfers  = "dealer_transfers"
	// Reviews moved from add-ons to standard (user decision 2026-10-07,
	// migration 000099).
	ModuleReviews = "reviews"

	// Add-ons (§5.3).
	ModuleAIAssistant    = "ai_assistant"
	ModuleWhatsApp       = "whatsapp_gateway"
	ModuleMCP            = "mcp"
	ModuleDealerShowcase = "dealer_showcase"
	ModuleFleet          = "fleet"
	ModuleCertificates   = "certificates"
	ModuleStockForecast  = "stock_forecast"
	ModulePerformance    = "performance"
	ModuleEfficiency     = "efficiency"
	ModuleCampaigns      = "campaigns"
	ModulePhotoStandard  = "photo_standard"
	ModuleEInvoice       = "e_invoice"
	ModuleShortURL       = "short_url"
)

func core(key string) ModuleDef { return ModuleDef{Key: key, Level: LevelCore, DefaultEnabled: true} }

func standard(key string) ModuleDef {
	return ModuleDef{Key: key, Level: LevelStandard, DefaultEnabled: true}
}

func addon(key string, paid bool) ModuleDef {
	return ModuleDef{Key: key, Level: LevelAddon, Paid: paid}
}

// Modules is the module catalog in display order.
var Modules = []ModuleDef{
	core(ModuleOrganizations),
	core(ModuleRegions),
	core(ModuleCatalog),
	core(ModuleStock),
	core(ModuleWarehouse),
	core(ModuleOrders),
	core(ModuleServices),
	core(ModuleCustomers),
	core(ModuleNotifications),
	core(ModuleAccounting),
	core(ModuleSearch),
	core(ModuleImportExport),
	core(ModuleTasks),
	core(ModuleSystemSettings),

	standard(ModuleIntakeContracts),
	standard(ModuleMeasurements),
	standard(ModuleLeads),
	standard(ModuleAppointments),
	standard(ModuleAnnouncements),
	standard(ModuleWarrantyClaims),
	standard(ModuleDealerAccounting),
	standard(ModuleServiceCatalog),
	standard(ModuleDealerTransfers),
	standard(ModuleReviews),

	addon(ModuleAIAssistant, true),
	addon(ModuleWhatsApp, true),
	addon(ModuleMCP, true),
	addon(ModuleDealerShowcase, true),
	addon(ModuleFleet, true),
	addon(ModuleCertificates, true),
	addon(ModuleStockForecast, true),
	addon(ModulePerformance, true),
	addon(ModuleEfficiency, true),
	addon(ModuleCampaigns, true),
	// Photo standard is an admin switch (§5.3 "admin aç/kapa"), not sold.
	addon(ModulePhotoStandard, false),
	addon(ModuleEInvoice, true),
	addon(ModuleShortURL, true),
}

var moduleIndex = map[string]int{}

func init() {
	for i, m := range Modules {
		moduleIndex[m.Key] = i
	}
}

// ModuleByKey returns a catalog entry.
func ModuleByKey(key string) (ModuleDef, bool) {
	i, ok := moduleIndex[key]
	if !ok {
		return ModuleDef{}, false
	}
	return Modules[i], true
}

// IsCore reports whether key is a core (always on) module.
func IsCore(key string) bool {
	m, ok := ModuleByKey(key)
	return ok && m.Level == LevelCore
}

// SortOrder is the display order of a module.
func (m ModuleDef) SortOrder() int32 {
	return int32((moduleIndex[m.Key] + 1) * 10)
}

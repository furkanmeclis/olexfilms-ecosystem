package i18n

// Label catalogs for the locales beyond tr/en (K10). They start empty: every
// key falls back to en until the translations land (TEC-138). Keep one map per
// locale here so check-i18n and translators have a fixed place to fill.
var (
	bgCatalog   = map[string]string{}
	deCatalog   = map[string]string{}
	elCatalog   = map[string]string{}
	ukCatalog   = map[string]string{}
	ruCatalog   = map[string]string{}
	frCatalog   = map[string]string{}
	esCatalog   = map[string]string{}
	itCatalog   = map[string]string{}
	zhCNCatalog = map[string]string{}
	azCatalog   = map[string]string{}
	arCatalog   = map[string]string{}
)

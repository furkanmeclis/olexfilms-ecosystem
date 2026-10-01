package i18n

// Label catalogs for the locales beyond tr/en (K10) that are not translated
// yet: every key falls back to en. A translated locale moves to its own
// catalog_<locale>.go file (TEC-138: de, fr, es, it, ru, uk), which
// check-i18n also reads.
var (
	bgCatalog   = map[string]string{}
	elCatalog   = map[string]string{}
	zhCNCatalog = map[string]string{}
	azCatalog   = map[string]string{}
	arCatalog   = map[string]string{}
)

package i18n

// Label catalogs for the locales beyond tr/en (K10). They start empty: every
// key falls back to en until the translations land (TEC-138). Keep one map per
// locale here so check-i18n and translators have a fixed place to fill.
//
// Translated locales live in their own file (catalog_<locale>.go):
// bg, el, zh-CN (catalog_zhcn.go), az, ar.
var (
	deCatalog = map[string]string{}
	ukCatalog = map[string]string{}
	ruCatalog = map[string]string{}
	frCatalog = map[string]string{}
	esCatalog = map[string]string{}
	itCatalog = map[string]string{}
)

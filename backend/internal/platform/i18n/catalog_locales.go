package i18n

// Label catalogs for the locales beyond tr/en (K10). Every translated locale
// has its own catalog_<locale>.go file (TEC-138: de, fr, es, it, ru, uk, bg,
// el, zh-CN in catalog_zhcn.go, az, ar), which check-i18n also reads. A new
// locale starts here as an empty map (every key falls back to en) and moves
// to its own file once translated.

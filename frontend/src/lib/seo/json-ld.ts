/**
 * TEC-1259: serializes JSON-LD for an inline `<script type="application/ld+json">`.
 *
 * JSON.stringify does not escape `<`, so a value containing `</script>` would
 * close the script element and inject HTML (stored XSS through a dealer name
 * or address). `<`, `>`, `&` and the JS line separators are written as
 * \u escapes; the JSON value stays identical for parsers.
 */
export function serializeJsonLd(value: unknown): string {
  return JSON.stringify(value)
    .replace(/</g, "\\u003c")
    .replace(/>/g, "\\u003e")
    .replace(/&/g, "\\u0026")
    .replace(/\u2028/g, "\\u2028")
    .replace(/\u2029/g, "\\u2029");
}

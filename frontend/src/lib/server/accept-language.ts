import { i18nConfig, normalizeLocale } from "@/config/i18n";

/**
 * Accept-Language the BFF sends to Go (TEC-137): the language picked in the
 * UI (NEXT_LOCALE cookie) first, then the browser's own header. Go uses it
 * after the user and organization settings (platform/i18n.Resolve).
 */
export function acceptLanguageFor(request: Request): string {
  const cookie = request.headers.get("cookie") ?? "";
  for (const part of cookie.split(";")) {
    const [name, ...rest] = part.trim().split("=");
    if (name !== i18nConfig.cookieName) continue;
    const locale = normalizeLocale(decodeURIComponent(rest.join("=")));
    if (locale) {
      const browser = request.headers.get("accept-language");
      return browser ? `${locale},${browser}` : locale;
    }
  }
  return request.headers.get("accept-language") || i18nConfig.defaultLocale;
}

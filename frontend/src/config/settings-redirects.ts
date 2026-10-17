/**
 * TEC-771: settings type pages moved under /platform/settings and
 * /t/{slug}/settings. Old paths (bookmarks, links in e-mails and
 * notifications) keep working through these redirects; next.config.ts
 * imports the list, so it stays plain data without path aliases.
 */
export type LegacyRedirect = {
  source: string;
  destination: string;
  permanent: boolean;
};

/** Old platform segment → new segment under /platform/settings. */
const PLATFORM_MOVES: Array<[string, string]> = [
  ["roles", "roles"],
  ["access", "access"],
  ["auth/settings", "auth"],
  ["integrations", "integrations"],
  ["legal-texts", "legal-texts"],
  ["mcp-clients", "mcp-clients"],
  ["ai", "ai"],
  ["modules", "modules"],
  ["system-settings", "system"],
  ["notification-center", "notification-templates"],
  ["vehicle-catalog", "vehicle-catalog"],
  ["service-catalog", "service-catalog"],
  ["certificate-types", "certificate-types"],
  ["photo-angles", "photo-angles"],
  ["part-consumption-expectations", "part-consumption-expectations"],
  ["review-questions", "review-questions"],
  ["territories", "territories"],
  ["document-templates", "document-templates"],
  ["contract-templates", "contract-templates"],
  ["imports", "imports"],
  ["exports", "exports"],
  ["activity", "activity"],
  ["logs", "logs"],
  ["storage", "storage"],
];

/** Old tenant segment → new segment under /t/{slug}/settings. */
const TENANT_MOVES: Array<[string, string]> = [
  ["features", "modules"],
  ["showcase", "showcase"],
  ["photo-standard", "photo-standard"],
  ["measurements/devices", "measurement-devices"],
  ["orders/delivery-methods", "delivery-methods"],
  ["ai-usage", "ai-usage"],
  ["imports", "imports"],
  ["exports", "exports"],
];

export const settingsRedirects: LegacyRedirect[] = [
  ...PLATFORM_MOVES.map(([from, to]) => ({
    source: `/platform/${from}/:path*`,
    destination: `/platform/settings/${to}/:path*`,
    permanent: true,
  })),
  ...TENANT_MOVES.map(([from, to]) => ({
    source: `/t/:slug/${from}/:path*`,
    destination: `/t/:slug/settings/${to}/:path*`,
    permanent: true,
  })),
];

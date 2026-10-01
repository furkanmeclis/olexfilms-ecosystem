/**
 * Fixed error-tracking module list (K17), mirrored from backend
 * internal/errtrack/module.go. Every event carries one as the `module` tag;
 * anything else is reported as `unknown`.
 */
export const MODULES = [
  "auth",
  "org",
  "catalog",
  "inventory",
  "order",
  "service",
  "warranty",
  "accounting",
  "notification",
  "whatsapp",
  "ai",
  "mcp",
  "migrator",
  "docs",
] as const;

export type Module = (typeof MODULES)[number] | "unknown";

const MODULE_SET: ReadonlySet<string> = new Set(MODULES);

/** Normalizes a free string to a fixed module (unknown → "unknown"). */
export function parseModule(value: string | null | undefined): Module {
  const v = (value ?? "").trim().toLowerCase();
  return MODULE_SET.has(v) ? (v as Module) : "unknown";
}

const SEGMENT_ALIASES: Record<string, Module> = {
  auth: "auth",
  login: "auth",
  register: "auth",
  profile: "auth",
  me: "auth",
  users: "auth",
  roles: "auth",
  permissions: "auth",
  access: "auth",
  sessions: "auth",
  org: "org",
  orgs: "org",
  organizations: "org",
  brand: "org",
  brands: "org",
  catalog: "catalog",
  products: "catalog",
  inventory: "inventory",
  stock: "inventory",
  warehouses: "inventory",
  order: "order",
  orders: "order",
  service: "service",
  services: "service",
  warranty: "warranty",
  warranties: "warranty",
  garanti: "warranty",
  accounting: "accounting",
  ledger: "accounting",
  notification: "notification",
  notifications: "notification",
  "notification-preferences": "notification",
  whatsapp: "whatsapp",
  wuzapi: "whatsapp",
  ai: "ai",
  mcp: "mcp",
  migrator: "migrator",
  docs: "docs",
  pdf: "docs",
  exports: "docs",
};

/** Path segments that carry only the audience / layout, not the module. */
const SCOPE_SEGMENTS = new Set([
  "api",
  "v1",
  "platform",
  "tenant",
  "public",
  "internal",
  "panel",
  "portal",
  "t",
  "hooks",
]);

/**
 * Derives the module from a page or API path:
 * /platform/organizations → org, /api/v1/tenant/orders/1 → order,
 * /t/{slug}/... keeps looking after the tenant slug.
 */
export function moduleFromPath(pathname: string | null | undefined): Module {
  const segments = (pathname ?? "").split(/[?#]/)[0].split("/").filter(Boolean);
  for (let i = 0; i < segments.length; i++) {
    const seg = segments[i].toLowerCase();
    if (SCOPE_SEGMENTS.has(seg)) {
      // /t/{tenantSlug}/...: skip the slug as well.
      if (seg === "t") i++;
      continue;
    }
    return SEGMENT_ALIASES[seg] ?? "unknown";
  }
  return "unknown";
}

import type { components } from "@/generated/api";
import { isPermissionScope, type PermissionScope } from "@/config/permissions";

export type PublicUser = components["schemas"]["PublicUser"];
export type LoginRequest = components["schemas"]["LoginRequest"];
export type Me = components["schemas"]["Me"];
export type Tokens = components["schemas"]["Tokens"];

export type RoleSummary = {
  uuid: string;
  name: string;
  slug: string;
  description?: string | null;
  is_system?: boolean;
};

/** Organization tree level (center -> distributor -> dealer). */
export type OrganizationType = "center" | "distributor" | "dealer";

export type OrganizationSummary = {
  uuid: string;
  slug: string;
  name: string;
  role: string;
  logo_url?: string | null;
  status: string;
  access_ends_at?: string | null;
  type?: OrganizationType;
  brand?: { slug: string; name?: string };
  parent?: { uuid: string; slug?: string; name: string } | null;
};

/** Client session identity — profile + RBAC. */
export type AuthUser = {
  uuid: string;
  email: string;
  name: string;
  surname: string;
  fullName: string;
  status: string;
  /** Effective language (Me.effective_locale): user -> org -> center. */
  locale: string;
  /** The user's own choice; null inherits from the organization. */
  ownLocale: string | null;
  /** Effective IANA time zone (Me.effective_timezone). */
  timeZone: string;
  /** The user's own time zone; null inherits from the organization. */
  ownTimeZone: string | null;
  isSuperAdmin: boolean;
  emailVerified: boolean;
  permissions: string[];
  /** Permission slug -> scope in the active organization (TEC-85). */
  grants: Record<string, PermissionScope>;
  roles: string[];
  /** Roles of the user in the active organization. */
  organizationRoles: string[];
  organizations: OrganizationSummary[];
  realtimeUserChannel?: string;
  realtimeEnabled?: boolean;
  impersonation?: {
    uuid: string;
    email: string;
    name: string;
    surname: string;
    fullName: string;
  };
};

export function mapMeToAuthUser(me: Me): AuthUser {
  const { user, roles, permissions, realtime, impersonation, organizations } =
    me;
  const grants: Record<string, PermissionScope> = {};
  for (const [slug, scope] of Object.entries(me.grants ?? {})) {
    if (typeof scope === "string" && isPermissionScope(scope)) {
      grants[slug] = scope;
    }
  }
  const ownLocale =
    typeof user.locale === "string" && user.locale ? user.locale : null;
  const ownTimeZone =
    typeof user.timezone === "string" && user.timezone ? user.timezone : null;
  const locale = me.effective_locale || ownLocale || "tr";
  const timeZone = me.effective_timezone || ownTimeZone || "Europe/Istanbul";
  const mapped: AuthUser = {
    uuid: user.uuid,
    email: user.email,
    name: user.name,
    surname: user.surname,
    fullName: `${user.name} ${user.surname}`.trim(),
    status: user.status,
    locale,
    ownLocale,
    timeZone,
    ownTimeZone,
    isSuperAdmin: Boolean(user.is_super_admin),
    emailVerified: Boolean(user.email_verified),
    permissions: permissions ?? [],
    grants,
    roles: roles ?? [],
    organizationRoles: me.organization_roles ?? [],
    organizations: (organizations ?? []).map((org) => ({
      uuid: org.uuid,
      slug: org.slug,
      name: org.name,
      role: org.role,
      logo_url: org.logo_url,
      status: org.status,
      access_ends_at: org.access_ends_at,
      type: org.type,
      brand: org.brand,
      parent: org.parent ?? null,
    })),
    realtimeUserChannel: realtime?.user_channel,
    realtimeEnabled: realtime?.enabled,
  };
  if (impersonation?.user) {
    const actor = impersonation.user;
    mapped.impersonation = {
      uuid: actor.uuid,
      email: actor.email,
      name: actor.name,
      surname: actor.surname,
      fullName: `${actor.name} ${actor.surname}`.trim(),
    };
  }
  return mapped;
}

export function hasPlatformPermission(
  user: AuthUser | null | undefined,
): boolean {
  if (!user) return false;
  if (user.isSuperAdmin || user.roles.includes("super_admin")) return true;
  return user.permissions.some((p) => p.startsWith("platform."));
}

export function isPlatformUser(user: AuthUser | null | undefined): boolean {
  return hasPlatformPermission(user);
}

export function isCmsUser(user: AuthUser | null | undefined): boolean {
  if (!user) return false;
  return !isPlatformUser(user) && user.organizations.length > 0;
}

export function primaryOrganizationSlug(
  user: AuthUser | null | undefined,
): string | null {
  if (!user?.organizations?.length) return null;
  return user.organizations[0]?.slug ?? null;
}

export function defaultHomeForUser(user: AuthUser): string {
  if (isPlatformUser(user)) return "/platform";
  const slug = primaryOrganizationSlug(user);
  if (slug) return `/t/${slug}`;
  return "/register";
}

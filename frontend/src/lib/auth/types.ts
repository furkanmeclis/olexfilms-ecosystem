import type { components } from "@/generated/api";

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

export type OrganizationSummary = {
  uuid: string;
  slug: string;
  name: string;
  role: string;
  logo_url?: string | null;
  status: string;
  access_ends_at?: string | null;
};

/** Client session identity — profile + RBAC. */
export type AuthUser = {
  uuid: string;
  email: string;
  name: string;
  surname: string;
  fullName: string;
  status: string;
  locale: string;
  isSuperAdmin: boolean;
  emailVerified: boolean;
  permissions: string[];
  roles: string[];
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
  const locale =
    typeof user.locale === "string" && user.locale ? user.locale : "tr";
  const mapped: AuthUser = {
    uuid: user.uuid,
    email: user.email,
    name: user.name,
    surname: user.surname,
    fullName: `${user.name} ${user.surname}`.trim(),
    status: user.status,
    locale,
    isSuperAdmin: Boolean(user.is_super_admin),
    emailVerified: Boolean(user.email_verified),
    permissions: permissions ?? [],
    roles: roles ?? [],
    organizations: (organizations ?? []).map((org) => ({
      uuid: org.uuid,
      slug: org.slug,
      name: org.name,
      role: org.role,
      logo_url: org.logo_url,
      status: org.status,
      access_ends_at: org.access_ends_at,
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

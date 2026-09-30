/**
 * Permission grouping helpers for matrix UX.
 * Local type — OpenAPI does not yet expose a Permission schema.
 */
export type Permission = {
  slug: string;
  name?: string;
  description?: string;
};

/**
 * Stable group keys for permission matrix UX.
 * Slug prefix → group (e.g. `platform.users.read` → `platform`).
 */
export const PERMISSION_GROUP_ORDER = [
  "platform",
  "auth",
  "notifications",
  "roles",
  "permissions",
  "system",
  "other",
] as const;

export type PermissionGroupKey = (typeof PERMISSION_GROUP_ORDER)[number];

export type PermissionGroup = {
  key: PermissionGroupKey | string;
  permissions: Permission[];
};

export function permissionGroupKey(slug: string): string {
  const [prefix] = slug.split(".");
  if (!prefix) return "other";
  return PERMISSION_GROUP_ORDER.includes(
    prefix as (typeof PERMISSION_GROUP_ORDER)[number],
  )
    ? prefix
    : prefix || "other";
}

export function groupPermissions(items: Permission[]): PermissionGroup[] {
  const map = new Map<string, Permission[]>();

  for (const permission of items) {
    const key = permissionGroupKey(permission.slug);
    const bucket = map.get(key) ?? [];
    bucket.push(permission);
    map.set(key, bucket);
  }

  for (const [, list] of map) {
    list.sort((a, b) => a.slug.localeCompare(b.slug));
  }

  const orderedKeys = [
    ...PERMISSION_GROUP_ORDER.filter((key) => map.has(key)),
    ...[...map.keys()]
      .filter(
        (key) =>
          !PERMISSION_GROUP_ORDER.includes(
            key as (typeof PERMISSION_GROUP_ORDER)[number],
          ),
      )
      .sort(),
  ];

  return orderedKeys.map((key) => ({
    key,
    permissions: map.get(key) ?? [],
  }));
}

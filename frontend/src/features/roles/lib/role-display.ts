import type { RoleSummary } from "@/features/roles/services/roles.service";

type Translate = (key: string) => string;

export function roleDisplayName(
  role: Pick<RoleSummary, "slug" | "name">,
  t: Translate,
) {
  const key = `roles.names.${role.slug}`;
  const label = t(key);
  return label === key ? role.name : label;
}

/**
 * TEC-1089: translated one-line summary of a system organization role
 * (`roles.descriptions.<slug>`); null when the role has none (custom roles),
 * so the untranslated backend description is never shown.
 */
export function roleDisplayDescription(
  role: Pick<RoleSummary, "slug">,
  t: Translate,
): string | null {
  const key = `roles.descriptions.${role.slug}`;
  const label = t(key);
  return label === key ? null : label;
}

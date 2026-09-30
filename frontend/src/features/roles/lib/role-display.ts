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

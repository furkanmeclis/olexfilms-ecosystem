import { EMPTY_NAV_ADORNMENT } from "@/features/nav-engine/define";
import type {
  NavAdornment,
  NavBadge,
  NavItemDef,
} from "@/features/nav-engine/types";

export function mergeNavAdornment(
  item: NavItemDef,
  dynamic: NavAdornment | undefined,
  soonLabel?: string,
): NavAdornment {
  const source = dynamic ?? EMPTY_NAV_ADORNMENT;
  const soon: NavBadge[] =
    item.soon && soonLabel
      ? [{ kind: "label", text: soonLabel, variant: "warning" }]
      : [];

  return {
    badges: [...(source.badges ?? []), ...(item.badges ?? []), ...soon],
    info: source.info ?? item.info ?? null,
  };
}

export function visibleCountTotal(badges: NavBadge[] | undefined): number {
  if (!badges?.length) return 0;
  return badges.reduce((sum, badge) => {
    if (badge.kind !== "count") return sum;
    if ((badge.hiddenWhenZero ?? true) && badge.value <= 0) return sum;
    return sum + badge.value;
  }, 0);
}

export function hasVisibleNavBadge(badges: NavBadge[] | undefined): boolean {
  if (!badges?.length) return false;
  return badges.some((badge) => {
    if (badge.kind === "count") {
      return !((badge.hiddenWhenZero ?? true) && badge.value <= 0);
    }
    if (badge.kind === "custom") return true;
    return true;
  });
}

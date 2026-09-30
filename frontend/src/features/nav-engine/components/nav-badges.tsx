"use client";

import { Badge } from "@/components/ui/badge";
import { formatNavCount } from "@/features/nav-engine/lib/active";
import type { NavBadge, NavBadgeVariant } from "@/features/nav-engine/types";
import { cn } from "@/lib/utils";

const DOT_CLASS: Record<NavBadgeVariant, string> = {
  default: "bg-primary",
  secondary: "bg-muted-foreground/70",
  success: "bg-emerald-500",
  warning: "bg-amber-500",
  danger: "bg-destructive",
  outline: "bg-border",
};

function isBadgeHidden(badge: NavBadge): boolean {
  if (badge.kind !== "count") return false;
  return (badge.hiddenWhenZero ?? true) && badge.value <= 0;
}

export function NavBadges({
  badges,
  className,
  compact = false,
}: {
  badges?: NavBadge[];
  className?: string;
  compact?: boolean;
}) {
  const visible = badges?.filter((badge) => !isBadgeHidden(badge));
  if (!visible?.length) return null;

  return (
    <span
      className={cn(
        "ms-auto flex shrink-0 items-center gap-1",
        compact && "gap-0.5",
        className,
      )}
    >
      {visible.map((badge, index) => (
        <NavBadgeView
          key={badgeKey(badge, index)}
          badge={badge}
          compact={compact}
        />
      ))}
    </span>
  );
}

function badgeKey(badge: NavBadge, index: number): string {
  if (badge.kind === "custom") return badge.id;
  if (badge.kind === "label") return `label:${badge.text}`;
  if (badge.kind === "count") return `count:${index}`;
  return `dot:${index}`;
}

function NavBadgeView({
  badge,
  compact,
}: {
  badge: NavBadge;
  compact: boolean;
}) {
  if (badge.kind === "custom") return badge.render();

  if (badge.kind === "dot") {
    return (
      <span
        className={cn(
          "size-2 shrink-0 rounded-full",
          DOT_CLASS[badge.variant ?? "default"],
        )}
        aria-hidden
      />
    );
  }

  if (badge.kind === "count") {
    return (
      <Badge
        variant={badge.variant ?? "secondary"}
        className={cn(
          "h-5 min-w-5 justify-center px-1 py-0 tabular-nums",
          compact ? "text-[9px]" : "text-[10px]",
        )}
      >
        {formatNavCount(badge.value, badge.max)}
      </Badge>
    );
  }

  return (
    <Badge
      variant={badge.variant ?? "secondary"}
      className={cn(
        "h-5 px-1.5 py-0 uppercase",
        compact ? "text-[9px]" : "text-[10px]",
      )}
    >
      {badge.text}
    </Badge>
  );
}

export function NavCollapsedIndicator({
  count,
  hasExtra,
  className,
}: {
  count: number;
  hasExtra: boolean;
  className?: string;
}) {
  if (count <= 0 && !hasExtra) return null;

  if (count > 0) {
    return (
      <span
        className={cn(
          "bg-primary text-primary-foreground pointer-events-none absolute -end-1 -top-1 z-10 flex h-4 min-w-4 items-center justify-center rounded-full px-0.5 text-[9px] font-medium tabular-nums",
          className,
        )}
      >
        {formatNavCount(count)}
      </span>
    );
  }

  return (
    <span
      className={cn(
        "bg-primary pointer-events-none absolute -end-0.5 -top-0.5 z-10 size-2 rounded-full",
        className,
      )}
    />
  );
}

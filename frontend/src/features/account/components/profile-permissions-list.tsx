"use client";

import { useMemo } from "react";

import { ALL_PERMISSIONS, isKnownPermission } from "@/config/permissions";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

type ProfilePermissionsListProps = {
  granted: string[] | undefined;
  className?: string;
};

function permissionLabelKey(slug: string) {
  return `permissions.labels.${slug}`;
}

/**
 * Read-only 3-column list of the user's permissions.
 * Labels come from i18n (`permissions.labels.*`); slug is kept as title.
 */
export function ProfilePermissionsList({
  granted,
  className,
}: ProfilePermissionsListProps) {
  const { t } = useLocale();

  const items = useMemo(() => {
    const set = new Set((granted ?? []).filter(Boolean));
    if (!set.size) return [];

    const known = ALL_PERMISSIONS.filter((slug) => set.has(slug));
    const unknown = [...set]
      .filter((slug) => !isKnownPermission(slug))
      .sort((a, b) => a.localeCompare(b));

    return [...known, ...unknown];
  }, [granted]);

  if (!items.length) {
    return (
      <span className="text-muted-foreground text-sm">
        {t("permissions.view_empty")}
      </span>
    );
  }

  return (
    <ul
      className={cn(
        "grid grid-cols-1 gap-x-4 gap-y-2 overflow-y-auto sm:grid-cols-2 lg:grid-cols-3",
        className,
      )}
    >
      {items.map((slug) => {
        const labelKey = permissionLabelKey(slug);
        const label = t(labelKey);
        const display = label === labelKey ? slug : label;

        return (
          <li
            key={slug}
            className="bg-muted/40 flex min-w-0 flex-col gap-0.5 rounded-md px-2.5 py-1.5"
            title={slug}
          >
            <span className="text-foreground truncate text-sm leading-snug">
              {display}
            </span>
            <span className="text-muted-foreground truncate font-mono text-[10px]">
              {slug}
            </span>
          </li>
        );
      })}
    </ul>
  );
}

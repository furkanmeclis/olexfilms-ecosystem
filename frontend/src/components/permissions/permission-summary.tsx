"use client";

import { PermissionBadge } from "@/components/permissions/permission-badge";
import type { Permission } from "@/components/permissions/permission-groups";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";

type PermissionSummaryProps = {
  selectedSlugs: string[];
  catalog?: Permission[];
  maxVisible?: number;
  className?: string;
};

/**
 * Compact selected-permission chips + counter (sticky header companion).
 */
export function PermissionSummary({
  selectedSlugs,
  catalog,
  maxVisible = 8,
  className,
}: PermissionSummaryProps) {
  const { t } = useLocale();
  const labelBySlug = new Map(
    (catalog ?? []).map((item) => [item.slug, item.name || item.slug]),
  );

  const visible = selectedSlugs.slice(0, maxVisible);
  const rest = Math.max(0, selectedSlugs.length - visible.length);

  return (
    <div
      className={cn(
        "border-border bg-card/95 supports-[backdrop-filter]:bg-card/80 sticky top-0 z-10 space-y-2 border-b pb-3 backdrop-blur",
        className,
      )}
    >
      <div className="flex items-center justify-between gap-2 text-sm">
        <span className="font-medium">{t("permissions.summary_title")}</span>
        <span className="text-muted-foreground">
          {t("permissions.summary_count", { count: selectedSlugs.length })}
        </span>
      </div>
      {selectedSlugs.length === 0 ? (
        <p className="text-muted-foreground text-xs">
          {t("permissions.summary_empty")}
        </p>
      ) : (
        <div className="flex flex-wrap gap-1.5">
          {visible.map((slug) => (
            <PermissionBadge
              key={slug}
              label={labelBySlug.get(slug) ?? slug}
              selected
            />
          ))}
          {rest > 0 ? (
            <PermissionBadge
              label={t("permissions.summary_more", { count: rest })}
            />
          ) : null}
        </div>
      )}
    </div>
  );
}

"use client";

import { useMemo, useState } from "react";

import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion";
import { Badge } from "@/components/ui/badge";
import { EmptyState } from "@/components/common/empty-state";
import {
  groupPermissions,
  type Permission,
} from "@/components/permissions/permission-groups";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";

type PermissionMatrixViewProps = {
  /** Effective permission slugs to display (read-only). */
  slugs: string[];
  /** Optional catalog for human-readable names. */
  catalog?: Permission[];
  emptyTitle?: string;
  emptyDescription?: string;
  className?: string;
};

function resolvePermissions(
  slugs: string[],
  catalog?: Permission[],
): Permission[] {
  const bySlug = new Map((catalog ?? []).map((item) => [item.slug, item]));
  return [...new Set(slugs)]
    .filter(Boolean)
    .sort((a, b) => a.localeCompare(b))
    .map((slug) => {
      const known = bySlug.get(slug);
      return (
        known ?? {
          id: 0,
          slug,
          name: slug,
        }
      );
    });
}

/**
 * Read-only grouped permission matrix for user/role detail pages.
 */
export function PermissionMatrixView({
  slugs,
  catalog,
  emptyTitle,
  emptyDescription,
  className,
}: PermissionMatrixViewProps) {
  const { t } = useLocale();
  const permissions = useMemo(
    () => resolvePermissions(slugs, catalog),
    [catalog, slugs],
  );
  const groups = useMemo(() => groupPermissions(permissions), [permissions]);
  // Groups start collapsed so the matrix stays short until the user opens one.
  const [openGroups, setOpenGroups] = useState<string[]>([]);
  const openValue = openGroups;

  const groupLabel = (key: string) => {
    const i18nKey = `permissions.groups.${key}`;
    const translated = t(i18nKey);
    return translated === i18nKey ? key : translated;
  };

  if (!permissions.length) {
    return (
      <EmptyState
        title={emptyTitle ?? t("permissions.view_empty_title")}
        description={emptyDescription ?? t("permissions.view_empty")}
        className="py-10"
      />
    );
  }

  return (
    <div className={cn("space-y-3", className)}>
      <div className="flex items-center justify-between gap-2 text-sm">
        <span className="text-muted-foreground">
          {t("permissions.view_count", { count: permissions.length })}
        </span>
        <span className="text-muted-foreground text-xs">
          {t("permissions.view_groups", { count: groups.length })}
        </span>
      </div>

      <Accordion
        type="multiple"
        value={openValue}
        onValueChange={setOpenGroups}
        className="space-y-2"
      >
        {groups.map((group) => (
          <AccordionItem
            key={group.key}
            value={group.key}
            className="border-border rounded-lg border px-3"
          >
            <AccordionTrigger className="py-3 hover:no-underline">
              <div className="flex min-w-0 flex-1 items-center gap-2 text-start">
                <span className="truncate font-medium">
                  {groupLabel(group.key)}
                </span>
                <Badge variant="secondary" className="font-normal tabular-nums">
                  {group.permissions.length}
                </Badge>
              </div>
            </AccordionTrigger>
            <AccordionContent>
              <ul className="grid gap-2 pb-1 sm:grid-cols-2 lg:grid-cols-3">
                {group.permissions.map((permission) => (
                  <li
                    key={permission.slug}
                    className="bg-muted/40 rounded-md px-2.5 py-2"
                  >
                    <p className="text-sm leading-snug font-medium">
                      {permission.name && permission.name !== permission.slug
                        ? permission.name
                        : permission.slug.split(".").slice(1).join(".") ||
                          permission.slug}
                    </p>
                    <p className="text-muted-foreground mt-0.5 truncate font-mono text-[11px]">
                      {permission.slug}
                    </p>
                  </li>
                ))}
              </ul>
            </AccordionContent>
          </AccordionItem>
        ))}
      </Accordion>
    </div>
  );
}

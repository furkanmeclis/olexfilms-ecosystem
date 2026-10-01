"use client";

import { Search, ChevronsUpDown, ChevronsDownUp } from "lucide-react";
import { useMemo, useState } from "react";

import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { PermissionGroupPanel } from "@/components/permissions/permission-group";
import {
  groupPermissions,
  type Permission,
} from "@/components/permissions/permission-groups";
import { PermissionSummary } from "@/components/permissions/permission-summary";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";

type PermissionSelectorProps = {
  permissions: Permission[];
  value: string[];
  onChange: (next: string[]) => void;
  disabled?: boolean;
  className?: string;
  /** When true, renders sticky summary counter */
  showSummary?: boolean;
  /** Grant scope per permission slug (TEC-85); enables the scope picker. */
  scopes?: Record<string, string>;
  onScopesChange?: (next: Record<string, string>) => void;
};

/**
 * Grouped permission matrix for role forms (and future dealer/workflow roles).
 */
export function PermissionSelector({
  permissions,
  value,
  onChange,
  disabled,
  className,
  showSummary = true,
  scopes,
  onScopesChange,
}: PermissionSelectorProps) {
  const { t } = useLocale();
  const [query, setQuery] = useState("");
  const selected = useMemo(() => new Set(value), [value]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return permissions;
    return permissions.filter(
      (item) =>
        item.slug.toLowerCase().includes(q) ||
        (item.name?.toLowerCase().includes(q) ?? false),
    );
  }, [permissions, query]);

  const groups = useMemo(() => groupPermissions(filtered), [filtered]);
  const allGroupKeys = groups.map((g) => g.key);
  const [openGroups, setOpenGroups] = useState<string[]>(allGroupKeys);

  const groupLabel = (key: string) => {
    const i18nKey = `permissions.groups.${key}`;
    const translated = t(i18nKey);
    return translated === i18nKey ? key : translated;
  };

  const setSelected = (next: Set<string>) => {
    onChange([...next].sort());
  };

  const selectable = (p: Permission) => !p.super_admin_only;

  const setScope = (slug: string, scope: string) => {
    if (!onScopesChange) return;
    onScopesChange({ ...(scopes ?? {}), [slug]: scope });
  };

  const toggle = (slug: string, next: boolean) => {
    const copy = new Set(selected);
    if (next) copy.add(slug);
    else copy.delete(slug);
    setSelected(copy);
  };

  const selectGroup = (slugs: string[]) => {
    const copy = new Set(selected);
    for (const slug of slugs) copy.add(slug);
    setSelected(copy);
  };

  const clearGroup = (slugs: string[]) => {
    const copy = new Set(selected);
    for (const slug of slugs) copy.delete(slug);
    setSelected(copy);
  };

  const selectAll = () => {
    setSelected(new Set(filtered.filter(selectable).map((p) => p.slug)));
  };

  const clearAll = () => {
    if (!query.trim()) {
      setSelected(new Set());
      return;
    }
    const filteredSlugs = new Set(filtered.map((p) => p.slug));
    const copy = new Set(selected);
    for (const slug of filteredSlugs) copy.delete(slug);
    setSelected(copy);
  };

  return (
    <div className={cn("space-y-3", className)}>
      {showSummary ? (
        <PermissionSummary selectedSlugs={value} catalog={permissions} />
      ) : null}

      <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
        <div className="relative min-w-0 flex-1">
          <Search className="text-muted-foreground absolute start-2.5 top-1/2 size-4 -translate-y-1/2" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t("permissions.search_placeholder")}
            className="ps-8"
            aria-label={t("permissions.search_placeholder")}
            disabled={disabled}
          />
        </div>
        <div className="flex flex-wrap items-center gap-1">
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={disabled}
            onClick={() => setOpenGroups(allGroupKeys)}
          >
            <ChevronsUpDown className="size-4" />
            {t("permissions.expand_all")}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={disabled}
            onClick={() => setOpenGroups([])}
          >
            <ChevronsDownUp className="size-4" />
            {t("permissions.collapse_all")}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="secondary"
            disabled={disabled || filtered.length === 0}
            onClick={selectAll}
          >
            {t("permissions.select_all")}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            disabled={disabled || value.length === 0}
            onClick={clearAll}
          >
            {t("permissions.clear_all")}
          </Button>
        </div>
      </div>

      {groups.length === 0 ? (
        <p className="text-muted-foreground py-8 text-center text-sm">
          {t("permissions.empty_filtered")}
        </p>
      ) : (
        <Accordion
          type="multiple"
          value={openGroups}
          onValueChange={setOpenGroups}
          className="border-border rounded-lg border px-3"
        >
          {groups.map((group) => (
            <AccordionItem key={group.key} value={group.key}>
              <AccordionTrigger>
                <span className="flex items-center gap-2">
                  {groupLabel(group.key)}
                  <span className="text-muted-foreground text-xs font-normal">
                    (
                    {
                      group.permissions.filter((p) => selected.has(p.slug))
                        .length
                    }
                    /{group.permissions.length})
                  </span>
                </span>
              </AccordionTrigger>
              <AccordionContent>
                <PermissionGroupPanel
                  groupKey={group.key}
                  groupLabel={groupLabel(group.key)}
                  permissions={group.permissions}
                  selected={selected}
                  disabled={disabled}
                  onToggle={toggle}
                  scopes={scopes}
                  onScopeChange={onScopesChange ? setScope : undefined}
                  onSelectGroup={() =>
                    selectGroup(
                      group.permissions.filter(selectable).map((p) => p.slug),
                    )
                  }
                  onClearGroup={() =>
                    clearGroup(group.permissions.map((p) => p.slug))
                  }
                />
              </AccordionContent>
            </AccordionItem>
          ))}
        </Accordion>
      )}
    </div>
  );
}

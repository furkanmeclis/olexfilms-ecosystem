"use client";

import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import type { Permission } from "@/components/permissions/permission-groups";
import { useLocale } from "@/providers/locale-provider";

type PermissionGroupProps = {
  groupKey: string;
  groupLabel: string;
  permissions: Permission[];
  selected: Set<string>;
  disabled?: boolean;
  onToggle: (slug: string, next: boolean) => void;
  onSelectGroup: () => void;
  onClearGroup: () => void;
};

export function PermissionGroupPanel({
  groupKey,
  groupLabel,
  permissions,
  selected,
  disabled,
  onToggle,
  onSelectGroup,
  onClearGroup,
}: PermissionGroupProps) {
  const { t } = useLocale();
  const selectedCount = permissions.filter((p) => selected.has(p.slug)).length;
  const allSelected =
    permissions.length > 0 && selectedCount === permissions.length;

  return (
    <div
      className="space-y-3"
      role="group"
      aria-labelledby={`perm-group-${groupKey}`}
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="space-y-0.5">
          <p id={`perm-group-${groupKey}`} className="text-sm font-medium">
            {groupLabel}
          </p>
          <p className="text-muted-foreground text-xs">
            {t("permissions.group_selected", {
              selected: selectedCount,
              total: permissions.length,
            })}
          </p>
        </div>
        <div className="flex items-center gap-1">
          <Button
            type="button"
            size="sm"
            variant="ghost"
            disabled={disabled || allSelected}
            onClick={onSelectGroup}
          >
            {t("permissions.select_group")}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            disabled={disabled || selectedCount === 0}
            onClick={onClearGroup}
          >
            {t("permissions.clear_group")}
          </Button>
        </div>
      </div>

      <ul className="grid gap-2 sm:grid-cols-2">
        {permissions.map((permission) => {
          const id = `perm-${permission.slug}`;
          const checked = selected.has(permission.slug);
          return (
            <li key={permission.slug} className="flex items-start gap-2">
              <Checkbox
                id={id}
                checked={checked}
                disabled={disabled}
                onCheckedChange={(value) =>
                  onToggle(permission.slug, value === true)
                }
              />
              <div className="min-w-0 space-y-0.5">
                <Label htmlFor={id} className="cursor-pointer leading-snug">
                  {permission.name || permission.slug}
                </Label>
                <p className="text-muted-foreground truncate font-mono text-[11px]">
                  {permission.slug}
                </p>
              </div>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

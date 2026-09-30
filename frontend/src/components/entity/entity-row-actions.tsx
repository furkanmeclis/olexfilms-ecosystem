"use client";

import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

import { PermissionGuard } from "@/components/common/permission-guard";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";

export type EntityRowAction = {
  id: string;
  label: string;
  icon?: LucideIcon;
  permission?: string | string[];
  variant?: "default" | "destructive";
  disabled?: boolean;
  separatorBefore?: boolean;
  onSelect: () => void;
};

type EntityRowActionsProps = {
  actions: EntityRowAction[];
  label?: string;
};

function ActionIconButton({ action }: { action: EntityRowAction }) {
  const Icon = action.icon;

  const button = (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className={cn(
            "size-8 shrink-0",
            action.variant === "destructive" &&
              "text-destructive hover:bg-destructive/10 hover:text-destructive",
          )}
          disabled={action.disabled}
          aria-label={action.label}
          onClick={() => action.onSelect()}
        >
          {Icon ? <Icon className="size-4" /> : action.label}
        </Button>
      </TooltipTrigger>
      <TooltipContent side="top">{action.label}</TooltipContent>
    </Tooltip>
  );

  if (!action.permission) return button;

  return (
    <PermissionGuard permission={action.permission}>{button}</PermissionGuard>
  );
}

/**
 * Inline icon row actions with tooltips for entity tables.
 */
export function EntityRowActions({ actions, label }: EntityRowActionsProps) {
  const { t } = useLocale();
  const visible = actions.filter(Boolean);
  if (!visible.length) return null;

  return (
    <div
      className="flex items-center justify-end gap-0.5 whitespace-nowrap"
      role="group"
      aria-label={label ?? t("common.actions")}
    >
      {visible.map((action) => (
        <ActionIconButton key={action.id} action={action} />
      ))}
    </div>
  );
}

export type EntityRowActionsRender = (node: ReactNode) => ReactNode;

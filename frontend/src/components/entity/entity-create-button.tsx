"use client";

import { Plus } from "lucide-react";

import { PermissionGuard } from "@/components/common/permission-guard";
import { Button } from "@/components/ui/button";
import { useLocale } from "@/providers/locale-provider";

type EntityCreateButtonProps = {
  onClick: () => void;
  label?: string;
  permission?: string | string[];
  size?: "default" | "sm" | "lg" | "icon";
};

/**
 * Primary create CTA for entity list pages — place in EntityPage `actions` (header).
 */
export function EntityCreateButton({
  onClick,
  label,
  permission,
  size = "sm",
}: EntityCreateButtonProps) {
  const { t } = useLocale();

  const button = (
    <Button type="button" size={size} onClick={onClick}>
      <Plus className="size-4" />
      {label ?? t("entity.create")}
    </Button>
  );

  if (!permission) return button;

  return <PermissionGuard permission={permission}>{button}</PermissionGuard>;
}

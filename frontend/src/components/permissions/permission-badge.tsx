"use client";

import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

type PermissionBadgeProps = {
  label: string;
  selected?: boolean;
  className?: string;
};

export function PermissionBadge({
  label,
  selected = false,
  className,
}: PermissionBadgeProps) {
  return (
    <Badge
      variant={selected ? "default" : "secondary"}
      className={cn("font-normal", className)}
    >
      {label}
    </Badge>
  );
}

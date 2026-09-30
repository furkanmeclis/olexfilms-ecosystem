"use client";

import type { ComponentProps, ReactNode } from "react";

import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

type ToolbarIconButtonProps = ComponentProps<typeof Button> & {
  label: string;
  children: ReactNode;
};

/** Outline icon-only toolbar control with tooltip label */
export function ToolbarIconButton({
  label,
  children,
  className,
  variant = "outline",
  size = "icon",
  ...props
}: ToolbarIconButtonProps) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant={variant}
          size={size}
          className={cn("size-8 shadow-none", className)}
          aria-label={label}
          {...props}
        >
          {children}
        </Button>
      </TooltipTrigger>
      <TooltipContent side="bottom">{label}</TooltipContent>
    </Tooltip>
  );
}

"use client";

import type { ReactNode } from "react";

import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { cn } from "@/lib/utils";

type AppDrawerProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  children?: ReactNode;
  footer?: ReactNode;
  side?: "left" | "right";
  /** Width preset for form / detail drawers */
  size?: "sm" | "md" | "lg" | "xl";
  className?: string;
};

const sizeClass: Record<NonNullable<AppDrawerProps["size"]>, string> = {
  sm: "sm:max-w-sm",
  md: "sm:max-w-md",
  lg: "sm:max-w-lg",
  xl: "sm:max-w-xl",
};

/** Drawer alias — left/right sheet for denser admin workflows */
export function AppDrawer({
  open,
  onOpenChange,
  title,
  description,
  children,
  footer,
  side = "right",
  size = "sm",
  className,
}: AppDrawerProps) {
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side={side}
        className={cn(
          "flex w-full flex-col overflow-hidden",
          sizeClass[size],
          className,
        )}
      >
        <SheetHeader className="shrink-0 pr-8 text-left">
          <SheetTitle>{title}</SheetTitle>
          {description ? (
            <SheetDescription>{description}</SheetDescription>
          ) : null}
        </SheetHeader>
        <div className="mt-4 min-h-0 flex-1 overflow-y-auto pb-4">
          {children}
        </div>
        {footer ? (
          <div className="border-border bg-background shrink-0 border-t pt-4">
            {footer}
          </div>
        ) : null}
      </SheetContent>
    </Sheet>
  );
}

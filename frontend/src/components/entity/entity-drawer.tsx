"use client";

import type { ReactNode } from "react";

import { AppDrawer } from "@/components/dialogs/app-drawer";

type EntityDrawerProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  children?: ReactNode;
  footer?: ReactNode;
  size?: "sm" | "md" | "lg" | "xl";
};

/** Standard entity drawer (create / edit / detail). */
export function EntityDrawer({
  open,
  onOpenChange,
  title,
  description,
  children,
  footer,
  size = "lg",
}: EntityDrawerProps) {
  return (
    <AppDrawer
      open={open}
      onOpenChange={onOpenChange}
      title={title}
      description={description}
      footer={footer}
      size={size}
    >
      {children}
    </AppDrawer>
  );
}

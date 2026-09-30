"use client";

import type { ReactNode } from "react";

import { RouteGuard } from "@/components/common/route-guard";
import { AppLayout } from "@/components/layout";

/** CMS shell — `/` */
export default function CmsLayout({ children }: { children: ReactNode }) {
  return (
    <RouteGuard mode="cms">
      <AppLayout variant="cms">{children}</AppLayout>
    </RouteGuard>
  );
}

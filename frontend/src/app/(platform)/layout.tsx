"use client";

import type { ReactNode } from "react";

import { RouteGuard } from "@/components/common/route-guard";
import { AppLayout } from "@/components/layout";

/** Platform management shell — `/platform` */
export default function PlatformLayout({ children }: { children: ReactNode }) {
  return (
    <RouteGuard mode="platform">
      <AppLayout variant="platform">{children}</AppLayout>
    </RouteGuard>
  );
}

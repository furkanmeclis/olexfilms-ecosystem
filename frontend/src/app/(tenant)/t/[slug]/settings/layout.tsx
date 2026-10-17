"use client";

import type { ReactNode } from "react";
import { useParams } from "next/navigation";

import { SettingsLayout } from "@/components/layout/settings-layout";

/** TEC-771: organization settings area with its sub-navigation. */
export default function TenantSettingsLayout({
  children,
}: {
  children: ReactNode;
}) {
  const params = useParams<{ slug: string }>();
  return (
    <SettingsLayout variant="tenant" tenantSlug={String(params.slug ?? "")}>
      {children}
    </SettingsLayout>
  );
}

"use client";

import type { ReactNode } from "react";
import { useParams, usePathname } from "next/navigation";

import { AppLayout } from "@/components/layout";
import { TenantOrganizationContext } from "@/features/organizations/components/tenant-organization-context";
import { TenantRouteGuard } from "@/features/organizations/components/tenant-route-guard";
import { TenantProvider } from "@/features/organizations/providers/tenant-provider";

export default function TenantLayout({ children }: { children: ReactNode }) {
  const params = useParams<{ slug: string }>();
  const slug = String(params.slug ?? "");
  const pathname = usePathname();

  // The login page manages its own TenantProvider and TenantRouteGuard (mode="guest").
  // Wrapping it with mode="tenant" here would return null for unauthenticated users
  // before they can even see the login form.
  if (pathname === `/t/${slug}/login`) {
    return children;
  }

  return (
    <TenantProvider slug={slug}>
      <TenantRouteGuard mode="tenant">
        <TenantOrganizationContext slug={slug}>
          <AppLayout variant="tenant" tenantSlug={slug}>
            {children}
          </AppLayout>
        </TenantOrganizationContext>
      </TenantRouteGuard>
    </TenantProvider>
  );
}

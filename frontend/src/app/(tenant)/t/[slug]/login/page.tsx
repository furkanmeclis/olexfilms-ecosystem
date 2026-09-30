"use client";

import { useParams } from "next/navigation";

import { TenantLoginForm } from "@/features/organizations/components/tenant-login-form";
import { TenantRouteGuard } from "@/features/organizations/components/tenant-route-guard";
import { TenantProvider } from "@/features/organizations/providers/tenant-provider";

export default function TenantLoginPage() {
  const params = useParams<{ slug: string }>();
  const slug = String(params.slug ?? "");

  return (
    <TenantProvider slug={slug}>
      <TenantRouteGuard mode="guest">
        <TenantLoginForm />
      </TenantRouteGuard>
    </TenantProvider>
  );
}

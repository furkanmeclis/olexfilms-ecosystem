"use client";

import { useParams } from "next/navigation";

import { AccountProfilePage } from "@/features/account/components/account-profile-page";

export default function TenantProfilePage() {
  const params = useParams<{ slug: string }>();
  const slug = String(params.slug ?? "");

  return <AccountProfilePage shell="tenant" tenantSlug={slug} />;
}

"use client";

import { useParams } from "next/navigation";

import { TenantOrganizationDetailPage } from "@/features/organizations/components/tenant-organization-detail-page";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  return (
    <TenantOrganizationDetailPage
      key={String(params.uuid ?? "")}
      slug={String(params.slug ?? "")}
      uuid={String(params.uuid ?? "")}
    />
  );
}

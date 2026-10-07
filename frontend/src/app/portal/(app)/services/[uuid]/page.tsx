import type { Metadata } from "next";

import { PortalServiceDetail } from "@/features/portal/components/portal-service-detail";

export const metadata: Metadata = { title: "Portal" };

export default async function PortalServiceDetailPage({
  params,
  searchParams,
}: {
  params: Promise<{ uuid: string }>;
  searchParams: Promise<{ source?: string | string[] }>;
}) {
  const { uuid } = await params;
  const { source } = await searchParams;
  return (
    <PortalServiceDetail
      uuid={uuid}
      reviewFromLink={source === "whatsapp_link"}
    />
  );
}

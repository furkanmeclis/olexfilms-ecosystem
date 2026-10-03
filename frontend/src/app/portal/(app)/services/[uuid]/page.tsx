import type { Metadata } from "next";

import { PortalServiceDetail } from "@/features/portal/components/portal-service-detail";

export const metadata: Metadata = { title: "Portal" };

export default async function PortalServiceDetailPage({
  params,
}: {
  params: Promise<{ uuid: string }>;
}) {
  const { uuid } = await params;
  return <PortalServiceDetail uuid={uuid} />;
}

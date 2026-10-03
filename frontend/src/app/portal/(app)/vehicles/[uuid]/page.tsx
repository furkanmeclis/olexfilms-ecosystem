import type { Metadata } from "next";

import { PortalVehicleDetail } from "@/features/portal/components/portal-vehicle-detail";

export const metadata: Metadata = { title: "Portal" };

export default async function PortalVehicleDetailPage({
  params,
}: {
  params: Promise<{ uuid: string }>;
}) {
  const { uuid } = await params;
  return <PortalVehicleDetail uuid={uuid} />;
}

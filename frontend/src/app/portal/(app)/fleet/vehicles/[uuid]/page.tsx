import type { Metadata } from "next";

import { PortalFleetVehicleDetail } from "@/features/portal/components/portal-fleet";

export const metadata: Metadata = { title: "Portal" };

export default async function PortalFleetVehicleDetailPage({
  params,
}: {
  params: Promise<{ uuid: string }>;
}) {
  const { uuid } = await params;
  return <PortalFleetVehicleDetail uuid={uuid} />;
}

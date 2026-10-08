import type { Metadata } from "next";

import { PortalFleetVehicles } from "@/features/portal/components/portal-fleet";

export const metadata: Metadata = { title: "Portal" };

export default function PortalFleetVehiclesPage() {
  return <PortalFleetVehicles />;
}

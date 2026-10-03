import type { Metadata } from "next";

import { PortalVehicles } from "@/features/portal/components/portal-vehicles";

export const metadata: Metadata = { title: "Portal" };

export default function PortalVehiclesPage() {
  return <PortalVehicles />;
}

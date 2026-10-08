import type { Metadata } from "next";

import { PortalFleetReports } from "@/features/portal/components/portal-fleet";

export const metadata: Metadata = { title: "Portal" };

export default function PortalFleetReportsPage() {
  return <PortalFleetReports />;
}

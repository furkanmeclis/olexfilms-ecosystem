import type { Metadata } from "next";

import { PortalFleetAccount } from "@/features/portal/components/portal-fleet";

export const metadata: Metadata = { title: "Portal" };

export default function PortalFleetAccountPage() {
  return <PortalFleetAccount />;
}

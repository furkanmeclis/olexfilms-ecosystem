import type { Metadata } from "next";

import { PortalFleetServices } from "@/features/portal/components/portal-fleet";

export const metadata: Metadata = { title: "Portal" };

export default function PortalFleetServicesPage() {
  return <PortalFleetServices />;
}

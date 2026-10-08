import type { Metadata } from "next";

import { PortalFleetWarranties } from "@/features/portal/components/portal-fleet";

export const metadata: Metadata = { title: "Portal" };

export default function PortalFleetWarrantiesPage() {
  return <PortalFleetWarranties />;
}

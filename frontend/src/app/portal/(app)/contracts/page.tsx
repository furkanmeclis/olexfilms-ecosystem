import type { Metadata } from "next";

import { PortalContracts } from "@/features/portal/components/portal-contracts";

export const metadata: Metadata = { title: "Portal" };

export default function PortalContractsPage() {
  return <PortalContracts />;
}

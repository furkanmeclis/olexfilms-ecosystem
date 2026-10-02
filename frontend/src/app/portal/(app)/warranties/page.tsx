import type { Metadata } from "next";

import { PortalWarranties } from "@/features/warranty/components/portal-warranties";

export const metadata: Metadata = { title: "Portal" };

export default function PortalWarrantiesPage() {
  return <PortalWarranties />;
}

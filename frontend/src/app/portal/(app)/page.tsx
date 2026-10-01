import type { Metadata } from "next";

import { PortalHome } from "@/features/portal/components/portal-home";

export const metadata: Metadata = { title: "Portal" };

export default function PortalHomePage() {
  return <PortalHome />;
}

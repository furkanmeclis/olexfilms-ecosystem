import type { Metadata } from "next";

import { PortalProfile } from "@/features/portal/components/portal-profile";

export const metadata: Metadata = { title: "Portal" };

export default function PortalProfilePage() {
  return <PortalProfile />;
}

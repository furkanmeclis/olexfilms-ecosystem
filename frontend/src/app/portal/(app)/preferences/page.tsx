import type { Metadata } from "next";

import { PortalPreferences } from "@/features/portal/components/portal-preferences";

export const metadata: Metadata = { title: "Portal" };

export default function PortalPreferencesPage() {
  return <PortalPreferences />;
}

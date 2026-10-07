import type { Metadata } from "next";

import { PortalAssistantPage } from "@/features/ai-assistant";

export const metadata: Metadata = { title: "Portal" };

export default function PortalAssistantRoute() {
  return <PortalAssistantPage />;
}

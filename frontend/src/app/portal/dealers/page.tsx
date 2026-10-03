import type { Metadata } from "next";

import { DealerFinder } from "@/features/dealers/components/dealer-finder";

/**
 * Dealer finder (TEC-242). Public: outside the portal `(app)` group, so no
 * portal session is needed; the landing page links here (TEC-247).
 */
export const metadata: Metadata = { title: "Portal" };

export default function PortalDealersPage() {
  return <DealerFinder />;
}

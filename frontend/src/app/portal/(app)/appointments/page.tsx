import type { Metadata } from "next";

import { PortalAppointments } from "@/features/portal/components/portal-appointments";

export const metadata: Metadata = { title: "Portal" };

export default function PortalAppointmentsPage() {
  return <PortalAppointments />;
}

import type { Metadata } from "next";

import { PortalAppointmentBooking } from "@/features/portal/components/portal-appointment-booking";
import { bookingPreset } from "@/features/portal/lib/portal-appointments";

export const metadata: Metadata = { title: "Portal" };

/** TEC-327: `?dealer=&dealer_name=` (dealer card) and `?vehicle=` preselect. */
export default async function PortalNewAppointmentPage({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const preset = bookingPreset(await searchParams);
  return (
    <PortalAppointmentBooking
      initialDealer={preset.dealer}
      initialVehicle={preset.vehicle}
    />
  );
}

"use client";

import { useParams } from "next/navigation";

import { AppointmentsPage } from "@/features/appointments";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <AppointmentsPage slug={String(params.slug ?? "")} />;
}

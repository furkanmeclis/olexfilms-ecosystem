"use client";

import { useParams } from "next/navigation";

import { AppointmentDetailPage } from "@/features/appointments";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  return (
    <AppointmentDetailPage
      key={String(params.uuid ?? "")}
      slug={String(params.slug ?? "")}
      uuid={String(params.uuid ?? "")}
    />
  );
}

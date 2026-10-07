"use client";

import { useParams } from "next/navigation";

import { MeasurementDevicesPage } from "@/features/measurements";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <MeasurementDevicesPage slug={String(params.slug ?? "")} />;
}

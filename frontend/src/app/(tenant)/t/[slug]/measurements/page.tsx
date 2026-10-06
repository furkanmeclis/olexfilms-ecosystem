"use client";

import { useParams } from "next/navigation";

import { MeasurementsListPage } from "@/features/measurements";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <MeasurementsListPage slug={String(params.slug ?? "")} />;
}

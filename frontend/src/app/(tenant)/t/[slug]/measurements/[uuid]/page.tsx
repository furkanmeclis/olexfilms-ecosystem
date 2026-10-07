"use client";

import { useParams } from "next/navigation";

import { MeasurementDetailPage } from "@/features/measurements";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  return (
    <MeasurementDetailPage
      key={String(params.uuid ?? "")}
      slug={String(params.slug ?? "")}
      uuid={String(params.uuid ?? "")}
    />
  );
}

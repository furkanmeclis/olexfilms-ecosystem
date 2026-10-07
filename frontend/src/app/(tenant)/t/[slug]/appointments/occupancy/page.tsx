"use client";

import { useParams } from "next/navigation";

import { OccupancyPage } from "@/features/appointments";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <OccupancyPage slug={String(params.slug ?? "")} />;
}

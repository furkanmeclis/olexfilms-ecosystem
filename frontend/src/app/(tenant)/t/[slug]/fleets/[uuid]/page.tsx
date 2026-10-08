"use client";

import { useParams } from "next/navigation";

import { FleetCardPage } from "@/features/fleets";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  return (
    <FleetCardPage
      key={String(params.uuid ?? "")}
      slug={String(params.slug ?? "")}
      uuid={String(params.uuid ?? "")}
    />
  );
}

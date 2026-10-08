"use client";

import { useParams } from "next/navigation";

import { FleetPlanDetailPage } from "@/features/fleets";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string; plan: string }>();
  return (
    <FleetPlanDetailPage
      key={String(params.plan ?? "")}
      slug={String(params.slug ?? "")}
      fleetUuid={String(params.uuid ?? "")}
      planUuid={String(params.plan ?? "")}
    />
  );
}

"use client";

import { useParams, useSearchParams } from "next/navigation";

import { FleetPlanWizard } from "@/features/fleets";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  const search = useSearchParams();
  const vehicles = (search.get("vehicles") ?? "")
    .split(",")
    .map((v) => v.trim())
    .filter(Boolean);
  return (
    <FleetPlanWizard
      key={String(params.uuid ?? "")}
      slug={String(params.slug ?? "")}
      fleetUuid={String(params.uuid ?? "")}
      initialVehicleUuids={vehicles}
    />
  );
}

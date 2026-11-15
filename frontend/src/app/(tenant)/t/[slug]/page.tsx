"use client";

import { useParams } from "next/navigation";

import { RoleOverview } from "@/features/overview";

/** TEC-1212: the overview by role (features/overview). */
export default function TenantHomePage() {
  const params = useParams<{ slug: string }>();
  return <RoleOverview slug={params.slug} />;
}

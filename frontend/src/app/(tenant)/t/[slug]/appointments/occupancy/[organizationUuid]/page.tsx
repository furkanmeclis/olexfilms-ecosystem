"use client";

import { useParams, useSearchParams } from "next/navigation";

import { OrganizationCalendarPage } from "@/features/appointments";

export default function Page() {
  const params = useParams<{ slug: string; organizationUuid: string }>();
  const search = useSearchParams();
  return (
    <OrganizationCalendarPage
      slug={String(params.slug ?? "")}
      organizationUuid={String(params.organizationUuid ?? "")}
      name={search?.get("name")}
      date={search?.get("date")}
      view={search?.get("view")}
    />
  );
}

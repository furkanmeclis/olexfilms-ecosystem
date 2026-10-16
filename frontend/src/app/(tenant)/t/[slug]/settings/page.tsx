"use client";

import { useParams } from "next/navigation";

import { SettingsIndexPage } from "@/components/layout/settings-layout";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return (
    <SettingsIndexPage
      variant="tenant"
      tenantSlug={String(params.slug ?? "")}
    />
  );
}

"use client";

import { useParams } from "next/navigation";

import { PhotoStandardSettingsPage } from "@/features/photo-standard";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <PhotoStandardSettingsPage slug={String(params.slug ?? "")} />;
}

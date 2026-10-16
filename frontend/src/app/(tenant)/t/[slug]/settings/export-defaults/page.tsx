"use client";

import { SettingsPage } from "@/features/io";
import { useParams } from "next/navigation";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <SettingsPage scope="tenant" slug={String(params.slug ?? "")} />;
}

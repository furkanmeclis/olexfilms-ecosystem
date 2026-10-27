"use client";

import { useParams } from "next/navigation";

import { PhotoAnglesPage } from "@/features/photo-standard";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <PhotoAnglesPage slug={String(params.slug ?? "")} />;
}

"use client";

import { useParams } from "next/navigation";

import { LibraryPage } from "@/features/library";

export default function TenantLibraryRoute() {
  const { slug } = useParams<{ slug: string }>();
  return <LibraryPage slug={slug} />;
}

"use client";

import { useParams } from "next/navigation";

import { DealerDirectoryPage } from "@/features/organizations/components/dealer-directory-page";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <DealerDirectoryPage slug={String(params.slug ?? "")} />;
}

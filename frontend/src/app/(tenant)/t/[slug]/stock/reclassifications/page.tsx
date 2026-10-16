"use client";

import { useParams } from "next/navigation";

import { ReclassificationsPage } from "@/features/stock";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <ReclassificationsPage slug={String(params.slug ?? "")} />;
}

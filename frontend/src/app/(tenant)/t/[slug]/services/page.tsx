"use client";

import { useParams } from "next/navigation";

import { ServicesListPage } from "@/features/services";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <ServicesListPage slug={String(params.slug ?? "")} />;
}

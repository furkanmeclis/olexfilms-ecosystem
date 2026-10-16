"use client";

import { useParams } from "next/navigation";

import { ReclassificationDetailPage } from "@/features/stock";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  return (
    <ReclassificationDetailPage
      slug={String(params.slug ?? "")}
      uuid={String(params.uuid ?? "")}
    />
  );
}

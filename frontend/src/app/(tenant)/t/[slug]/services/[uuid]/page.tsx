"use client";

import { useParams } from "next/navigation";

import { ServiceDetailPage } from "@/features/services";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  return (
    <ServiceDetailPage
      key={String(params.uuid ?? "")}
      slug={String(params.slug ?? "")}
      uuid={String(params.uuid ?? "")}
    />
  );
}

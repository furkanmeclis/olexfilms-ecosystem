"use client";

import { useParams } from "next/navigation";

import { ProductDetailPage } from "@/features/catalog";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  return (
    <ProductDetailPage
      slug={String(params.slug ?? "")}
      uuid={String(params.uuid ?? "")}
    />
  );
}

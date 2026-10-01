"use client";

import { useParams } from "next/navigation";

import { ProductFormPage } from "@/features/catalog";

export default function Page() {
  const params = useParams<{ slug: string; uuid: string }>();
  return (
    <ProductFormPage
      slug={String(params.slug ?? "")}
      uuid={String(params.uuid ?? "")}
    />
  );
}

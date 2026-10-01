"use client";

import { useParams } from "next/navigation";

import { ProductFormPage } from "@/features/catalog";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <ProductFormPage slug={String(params.slug ?? "")} />;
}

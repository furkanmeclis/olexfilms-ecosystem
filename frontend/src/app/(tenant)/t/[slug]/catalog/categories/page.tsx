"use client";

import { useParams } from "next/navigation";

import { CategoriesPage } from "@/features/catalog";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <CategoriesPage slug={String(params.slug ?? "")} />;
}

"use client";

import { ExportsPage } from "@/features/io";
import { useParams } from "next/navigation";

export default function Page() {
  const params = useParams<{ slug: string }>();
  return <ExportsPage scope="tenant" slug={String(params.slug ?? "")} />;
}
